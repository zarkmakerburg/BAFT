//go:build unix

package ingressauthority

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

var ErrReserved = errors.New("ingress numeric TCP port is already reserved")

// ReservationStore persists desired BAFT claims. It does NOT authorize host
// effects; stopped foreign-service inventory and signed command admission
// remain independent prerequisites for minting any live executor lease.
type ReservationStore struct {
	dir      string
	identity os.FileInfo
}

const reservationFilename = "pending.json"
const reservationMaxBytes = 1 << 20

func OpenReservationStore(dir string) (*ReservationStore, error) {
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return nil, ErrBinding
	}
	s := &ReservationStore{dir: dir}
	err := s.withLock(func(d *os.File, root *os.Root) error {
		info, err := d.Stat()
		if err == nil {
			s.identity = info
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return s, nil
}

func reservationKey(b BindingV1) string {
	return b.NodeID + "\x00tcp\x00" + strconv.Itoa(int(b.Port))
}

func (s *ReservationStore) withLock(run func(*os.File, *os.Root) error) error {
	if s == nil {
		return ErrUnverified
	}
	d, err := openPinnedAuthorityDir(s.dir, s.identity)
	if err != nil {
		return err
	}
	defer d.Close()
	if err := syscall.Flock(int(d.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(d.Fd()), syscall.LOCK_UN)
	info, err := d.Stat()
	if err != nil {
		return err
	}
	pathInfo, err := os.Lstat(s.dir)
	if err != nil || !os.SameFile(info, pathInfo) || !info.IsDir() || info.Mode().Perm() != 0700 ||
		info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 ||
		s.identity != nil && !os.SameFile(s.identity, info) {
		return ErrUnverified
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(st.Uid) != os.Geteuid() {
		return ErrUnverified
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := checkAuthorityRoot(d, root, s.dir, s.identity); err != nil {
		return err
	}
	if err := run(d, root); err != nil {
		return err
	}
	latest, err := os.Lstat(s.dir)
	if err != nil || !os.SameFile(info, latest) {
		return ErrUnverified
	}
	return nil
}

// ReservePending is an atomic cross-process *pending reservation* only.
// Idempotent restart readback is permitted but never implies live ownership.
func (s *ReservationStore) ReservePending(binding BindingV1) error {
	if err := binding.Validate(); err != nil {
		return err
	}
	return s.withLock(func(d *os.File, root *os.Root) error {
		claims, err := loadPending(root)
		if err != nil {
			return err
		}
		key := reservationKey(binding)
		if old, exists := claims[key]; exists {
			// The leftover of a release that crashed after its tombstone is
			// finished first; the new binding is then judged by the
			// monotonic release rules like any other.
			stale, err := staleReleasedClaim(root, key, old)
			if err != nil {
				return err
			}
			if !stale {
				if old == binding {
					return nil
				}
				return ErrReserved
			}
			delete(claims, key)
			if err := savePending(d, root, claims); err != nil {
				return err
			}
		}
		released, err := loadReleased(root)
		if err != nil {
			return err
		}
		if r, ok := released[key]; ok {
			if err := admitAfterRelease(r, binding); err != nil {
				return err
			}
		}
		// A tunnel/instance can hold only one unresolved lease, even when
		// a later signed operation selects another numeric TCP port.
		// Generation advancement needs a separately verified retirement.
		for _, old := range claims {
			if old.NodeID == binding.NodeID && old.Spec.TunnelID == binding.Spec.TunnelID &&
				old.Spec.InstanceID == binding.Spec.InstanceID {
				return ErrReserved
			}
		}
		claims[key] = binding
		return savePending(d, root, claims)
	})
}

func loadPending(root *os.Root) (map[string]BindingV1, error) {
	claims := make(map[string]BindingV1)
	f, err := root.OpenFile(reservationFilename, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if errors.Is(err, os.ErrNotExist) {
		return claims, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 ||
		int(st.Uid) != os.Geteuid() || st.Nlink != 1 || info.Size() < 2 || info.Size() > reservationMaxBytes {
		return nil, ErrUnverified
	}
	raw, err := io.ReadAll(io.LimitReader(f, reservationMaxBytes+1))
	if err != nil || len(raw) > reservationMaxBytes {
		return nil, ErrUnverified
	}
	if err := json.Unmarshal(raw, &claims); err != nil || claims == nil {
		return nil, ErrUnverified
	}
	canonical, err := json.Marshal(claims)
	if err != nil || !bytes.Equal(canonical, raw) {
		return nil, ErrUnverified
	}
	for key, b := range claims {
		if err := b.Validate(); err != nil || key != reservationKey(b) {
			return nil, ErrUnverified
		}
	}
	return claims, nil
}

func savePending(d *os.File, root *os.Root, claims map[string]BindingV1) error {
	raw, err := json.Marshal(claims)
	if err != nil {
		return err
	}
	if len(raw) > reservationMaxBytes {
		return ErrUnverified
	}
	f, err := root.OpenFile(".pending-write", os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(".pending-write")
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := root.Rename(".pending-write", reservationFilename); err != nil {
		return err
	}
	if err := d.Sync(); err != nil {
		return fmt.Errorf("reservation durability uncertain: %w", err)
	}
	return nil
}
