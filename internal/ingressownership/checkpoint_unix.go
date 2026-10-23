//go:build unix

package ingressownership

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// Checkpoint is an offline LAB-only high-water backup. It does not journal
// resource mutations or authorize adopting host listeners or firewall rules.
type checkpointRecord struct {
	Version  int           `json:"version"`
	Snapshot FenceSnapshot `json:"snapshot"`
	Digest   [32]byte      `json:"digest"`
}

func encodeCheckpoint(s FenceSnapshot) ([]byte, error) {
	if _, err := RecoverFences(s); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	return json.Marshal(checkpointRecord{Version: 1, Snapshot: s, Digest: sha256.Sum256(raw)})
}

// SaveCheckpoint requires a trusted pre-existing private directory. It writes
// an atomic, fsynced checkpoint; no production service uses this entry point.
func SaveCheckpoint(dir string, s FenceSnapshot) error {
	raw, err := encodeCheckpoint(s)
	if err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	// Serialize writers across processes before reading the previous high-water
	// checkpoint. Atomic rename alone does not prevent concurrent rollback.
	if err := syscall.Flock(int(d.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(d.Fd()), syscall.LOCK_UN)
	info, err := d.Stat()
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm() != 0700 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return ErrInvalid
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return ErrInvalid
	}
	// Refuse a replaced directory path; otherwise a rename could write into
	// an untrusted location while the open directory handle remains private.
	pathInfo, err := os.Lstat(dir)
	if err != nil || !os.SameFile(info, pathInfo) {
		return ErrInvalid
	}
	f, err := os.CreateTemp(dir, ".fence-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	// Never overwrite an unsafe pre-existing destination (symlink, hardlink,
	// wrong owner or permissions), even though rename itself is atomic.
	destination := filepath.Join(dir, "fences.json")
	if existing, statErr := os.Lstat(destination); statErr == nil {
		st, ok := existing.Sys().(*syscall.Stat_t)
		if !ok || !existing.Mode().IsRegular() || existing.Mode().Perm() != 0600 || st.Nlink != 1 || int(st.Uid) != os.Geteuid() {
			return ErrInvalid
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	// Checkpoints must never lower a persisted generation high-water mark.
	// This is an offline LAB guard, not a concurrent transaction journal.
	if _, statErr := os.Lstat(destination); statErr == nil {
		previous, loadErr := LoadCheckpoint(dir)
		if loadErr != nil {
			return loadErr
		}
		previousSnapshot, snapshotErr := previous.Snapshot()
		if snapshotErr != nil {
			return snapshotErr
		}
		current := make(map[string]uint64, len(s.Generations))
		for _, g := range s.Generations {
			current[g.TunnelID+"\x00"+g.InstanceID] = g.Generation
		}
		for _, g := range previousSnapshot.Generations {
			if current[g.TunnelID+"\x00"+g.InstanceID] < g.Generation {
				return ErrStale
			}
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	if err = os.Rename(name, destination); err != nil {
		return err
	}
	return d.Sync()
}

// LoadCheckpoint fails closed on missing, tampered, oversized or unsafe files.
func LoadCheckpoint(dir string) (*Registry, error) {
	d, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer d.Close()
	info, err := d.Stat()
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm() != 0700 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return nil, ErrInvalid
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return nil, ErrInvalid
	}
	pathInfo, err := os.Lstat(dir)
	if err != nil || !os.SameFile(info, pathInfo) {
		return nil, ErrInvalid
	}
	f, err := os.OpenFile(filepath.Join(dir, "fences.json"), os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	fs, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || int(fs.Uid) != os.Geteuid() || fs.Nlink != 1 || !fi.Mode().IsRegular() || fi.Mode().Perm() != 0600 || fi.Size() > 1<<20 {
		return nil, ErrInvalid
	}
	raw, err := io.ReadAll(io.LimitReader(f, 1<<20+1))
	if err != nil {
		return nil, err
	}
	var rec checkpointRecord
	if err = json.Unmarshal(raw, &rec); err != nil {
		return nil, fmt.Errorf("invalid LAB checkpoint: %w", err)
	}
	// Reject unknown/duplicate/noncanonical JSON before trusting its digest.
	encoded, err := json.Marshal(rec)
	if err != nil || !bytes.Equal(raw, encoded) {
		return nil, ErrInvalid
	}
	if rec.Version != 1 {
		return nil, ErrInvalid
	}
	canonical, err := json.Marshal(rec.Snapshot)
	if err != nil {
		return nil, err
	}
	if sha256.Sum256(canonical) != rec.Digest {
		return nil, errors.Join(ErrInvalid, errors.New("checkpoint digest mismatch"))
	}
	return RecoverFences(rec.Snapshot)
}
