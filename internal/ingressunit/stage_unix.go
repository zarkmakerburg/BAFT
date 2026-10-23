//go:build unix

package ingressunit

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// Stage creates an owned unit file in a trusted systemd directory. It does
// not reload systemd, enable the service, or start a listener. Its caller
// must independently authorize the manifest proof before calling it.
func Stage(directory string, spec ingressmanifest.Spec, proof ingressmanifest.Proof, listenerID, baftBinary, serviceUser string) (Candidate, bool, error) {
	candidate, err := Render(spec, proof, listenerID, baftBinary, serviceUser)
	if err != nil {
		return Candidate{}, false, err
	}
	if err := trustedDirectory(directory); err != nil {
		return Candidate{}, false, err
	}
	lock, err := os.Open(directory)
	if err != nil {
		return Candidate{}, false, err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return Candidate{}, false, err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	if err := trustedDirectory(directory); err != nil {
		return Candidate{}, false, err
	}
	pathInfo, err := os.Lstat(directory)
	if err != nil {
		return Candidate{}, false, err
	}
	openedInfo, err := lock.Stat()
	if err != nil || !os.SameFile(pathInfo, openedInfo) {
		return Candidate{}, false, errors.New("ingress unit directory changed while locking")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return Candidate{}, false, err
	}
	defer root.Close()
	rootInfo, err := root.Stat(".")
	if err != nil || !os.SameFile(openedInfo, rootInfo) {
		return Candidate{}, false, errors.New("ingress unit directory changed while opening")
	}
	if existing, err := readOwnedUnit(root, candidate.Name); err == nil {
		if MatchesOwnedBytes(candidate, existing) {
			currentPath, statErr := os.Lstat(directory)
			if statErr != nil || !os.SameFile(openedInfo, currentPath) {
				return Candidate{}, false, errors.New("ingress unit directory changed after inspection")
			}
			return candidate, false, nil
		}
		return Candidate{}, false, errors.New("ingress unit already exists with different bytes")
	} else if !errors.Is(err, os.ErrNotExist) {
		return Candidate{}, false, err
	}

	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return Candidate{}, false, err
	}
	tmpName := ".baft-ingress-stage-" + hex.EncodeToString(nonce[:])
	tmp, err := root.OpenFile(tmpName, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return Candidate{}, false, err
	}
	defer root.Remove(tmpName)
	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		return Candidate{}, false, err
	}
	if _, err := tmp.Write(candidate.Bytes); err != nil {
		tmp.Close()
		return Candidate{}, false, err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return Candidate{}, false, err
	}
	if err := tmp.Close(); err != nil {
		return Candidate{}, false, err
	}
	// Hard-link creation is atomic and refuses an existing name, including a
	// symlink. Never rename over a service unit owned by another actor.
	if err := root.Link(tmpName, candidate.Name); err != nil {
		if errors.Is(err, os.ErrExist) {
			if existing, readErr := readOwnedUnit(root, candidate.Name); readErr == nil && MatchesOwnedBytes(candidate, existing) {
				return candidate, false, nil
			}
		}
		return Candidate{}, false, fmt.Errorf("create ingress unit without replacement: %w", err)
	}
	if err := root.Remove(tmpName); err != nil {
		return Candidate{}, true, fmt.Errorf("remove ingress staging link: %w", err)
	}
	if err := lock.Sync(); err != nil {
		return Candidate{}, true, fmt.Errorf("sync staged ingress unit directory: %w", err)
	}
	pathInfo, err = os.Lstat(directory)
	if err != nil || !os.SameFile(openedInfo, pathInfo) {
		return Candidate{}, true, errors.New("ingress unit directory changed after staging")
	}
	return candidate, true, nil
}

func trustedDirectory(directory string) error {
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory || strings.ContainsRune(directory, '\x00') {
		return errors.New("ingress unit directory must be a clean absolute path")
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || stat.Uid != uint32(os.Geteuid()) || info.Mode().Perm()&0022 != 0 {
		return errors.New("ingress unit directory must be owned and not group or world writable")
	}
	return nil
}

func readOwnedUnit(root *os.Root, name string) ([]byte, error) {
	f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 || info.Mode().Perm() != 0644 {
		return nil, errors.New("ingress unit has unsafe ownership, type, links or mode")
	}
	return io.ReadAll(io.LimitReader(f, 16<<10))
}
