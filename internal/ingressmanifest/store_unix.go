//go:build unix

package ingressmanifest

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"syscall"
)

// Store requires a pre-existing trusted directory owned by the effective UID
// with mode0700. Its parent path must be trusted; no directory path comes from
// a job or manifest. Same-UID/root admission is outside this filesystem primitive.
// os.Root anchors later operations even if the directory is renamed.
type Store struct {
	mu        sync.Mutex
	root      *os.Root
	directory *os.File
	closed    bool
	fault     func(string) error // fault-injection only; nil in production
}

func Open(directory string) (*Store, error) {
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("ingress manifest directory cannot be a symlink")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	f, err := root.OpenFile(".", os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		root.Close()
		return nil, err
	}
	info, err = f.Stat()
	if err == nil {
		err = privateOwned(info, true)
	}
	if err != nil {
		f.Close()
		root.Close()
		return nil, err
	}
	return &Store{root: root, directory: f}, nil
}

func (s *Store) Close() error {
	if s == nil {
		return os.ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.directory == nil || s.root == nil {
		return os.ErrInvalid
	}
	err := s.directory.Close()
	if rootErr := s.root.Close(); err == nil {
		err = rootErr
	}
	return err
}

func privateOwned(info os.FileInfo, directory bool) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return errors.New("ingress manifest path owner mismatch")
	}
	forbidden := os.ModeSymlink | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
	if info.Mode()&forbidden != 0 {
		return errors.New("invalid ingress manifest file type or mode")
	}
	if directory {
		if !info.IsDir() || info.Mode().Perm() != 0700 {
			return errors.New("ingress manifest directory must have mode0700")
		}
	} else if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || stat.Nlink != 1 {
		return errors.New("ingress manifest files must be private, regular and singly linked")
	}
	return nil
}

func (s *Store) openFile(name string, flags int) (*os.File, error) {
	f, err := s.root.OpenFile(name, flags|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err == nil {
		err = privateOwned(info, false)
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func (s *Store) lock(write bool) (func(), error) {
	flags := os.O_RDONLY
	if write {
		created, err := s.root.OpenFile(".lock", os.O_CREATE|os.O_EXCL|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
		if err == nil {
			if err = created.Chmod(0600); err == nil {
				err = created.Sync()
			}
			created.Close()
			if err == nil {
				err = s.directory.Sync()
			}
			if err != nil {
				return nil, err
			}
		} else if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		flags = os.O_RDWR
	}
	f, err := s.openFile(".lock", flags)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotPrepared
	}
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || info.Size() != 0 {
		f.Close()
		return nil, errors.New("invalid ingress manifest lock file")
	}
	operation := syscall.LOCK_SH
	if write {
		operation = syscall.LOCK_EX
	}
	if err := syscall.Flock(int(f.Fd()), operation|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrBusy
		}
		return nil, err
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}

func (s *Store) read(name string) ([]byte, error) {
	f, err := s.openFile(name, os.O_RDONLY)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotPrepared
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxManifestBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxManifestBytes {
		return nil, errors.New("ingress manifest exceeds size limit")
	}
	return raw, nil
}

func (s *Store) inject(point string) error {
	if s.fault != nil {
		return s.fault(point)
	}
	return nil
}

// Prepare writes only a staged manifest. New resources start at generation1;
// advancing requires the exact previous caller-held proof and increments by1.
// An identical retry fsyncs the existing file and directory before succeeding.
func (s *Store) Prepare(spec Spec, previous *Proof) (Proof, error) {
	if s == nil {
		return Proof{}, os.ErrInvalid
	}
	raw, proof, err := encode(spec)
	if err != nil {
		return Proof{}, err
	}
	if previous != nil {
		if err := validateProof(*previous); err != nil {
			return Proof{}, err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.root == nil || s.directory == nil {
		return Proof{}, os.ErrClosed
	}
	unlock, err := s.lock(true)
	if err != nil {
		return Proof{}, err
	}
	defer unlock()
	name := filename(proof.TunnelID, proof.InstanceID)
	existing, err := s.read(name)
	switch {
	case err == nil:
		_, old, err := decode(existing)
		if err != nil {
			return Proof{}, err
		}
		if old.TunnelID != proof.TunnelID || old.InstanceID != proof.InstanceID {
			return Proof{}, errors.New("stored ingress manifest belongs to another owner tuple")
		}
		if bytes.Equal(raw, existing) {
			f, err := s.openFile(name, os.O_RDONLY)
			if err != nil {
				return Proof{}, err
			}
			err = f.Sync()
			f.Close()
			if err == nil {
				err = s.directory.Sync()
			}
			if err != nil {
				return proof, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
			}
			return proof, nil
		}
		if previous == nil || old != *previous || proof.Generation != old.Generation+1 {
			return Proof{}, errors.New("ingress generation or previous proof mismatch")
		}
	case errors.Is(err, ErrNotPrepared):
		if previous != nil || proof.Generation != 1 {
			return Proof{}, errors.New("new ingress manifests must begin at generation1 without previous proof")
		}
	default:
		return Proof{}, err
	}
	entropy := make([]byte, 16)
	if _, err := rand.Read(entropy); err != nil {
		return Proof{}, err
	}
	stage := ".stage-" + hex.EncodeToString(entropy)
	f, err := s.root.OpenFile(stage, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return Proof{}, err
	}
	defer func() { _ = f.Close(); _ = s.root.Remove(stage) }()
	if err := f.Chmod(0600); err != nil {
		return Proof{}, err
	}
	if _, err := f.Write(raw); err != nil {
		return Proof{}, err
	}
	if err := s.inject("before-file-sync"); err != nil {
		return Proof{}, err
	}
	if err := f.Sync(); err != nil {
		return Proof{}, err
	}
	if err := f.Close(); err != nil {
		return Proof{}, err
	}
	if err := s.inject("before-rename"); err != nil {
		return Proof{}, err
	}
	if err := s.root.Rename(stage, name); err != nil {
		return Proof{}, err
	}
	if err := s.inject("before-directory-sync"); err != nil {
		return proof, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
	}
	if err := s.directory.Sync(); err != nil {
		return proof, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
	}
	return proof, nil
}

// Inspect reads only, requires an exact independently retained Proof and never
// creates a lock/file or infers that a listener or firewall resource is owned.
func (s *Store) Inspect(expected Proof) (Inspection, error) {
	if s == nil {
		return Inspection{}, os.ErrInvalid
	}
	if err := validateProof(expected); err != nil {
		return Inspection{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.root == nil || s.directory == nil {
		return Inspection{}, os.ErrClosed
	}
	unlock, err := s.lock(false)
	if err != nil {
		return Inspection{}, err
	}
	defer unlock()
	raw, err := s.read(filename(expected.TunnelID, expected.InstanceID))
	if err != nil {
		return Inspection{}, err
	}
	manifest, actual, err := decode(raw)
	if err != nil {
		return Inspection{}, err
	}
	if actual != expected {
		return Inspection{}, errors.New("ingress prepared manifest proof mismatch")
	}
	return Inspection{Manifest: manifest, Proof: actual, PreparedBytesMatch: true}, nil
}
