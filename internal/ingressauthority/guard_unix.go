//go:build unix

package ingressauthority

import (
	"errors"
	"os"
	"syscall"
)

var ErrGuardBusy = errors.New("ingress authority mutation lock busy")

type PendingGuard struct {
	fd      *os.File
	root    *os.Root
	store   *ReservationStore
	binding BindingV1
}

func (s *ReservationStore) HoldPending(b BindingV1) (*PendingGuard, error) {
	if s == nil {
		return nil, ErrUnverified
	}
	if err := b.Validate(); err != nil {
		return nil, err
	}
	fd, err := os.Open(s.dir)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(fd.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		fd.Close()
		return nil, ErrGuardBusy
	}
	release := func() { syscall.Flock(int(fd.Fd()), syscall.LOCK_UN); fd.Close() }
	info, err := fd.Stat()
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 ||
		!os.SameFile(info, s.identity) {
		release()
		return nil, ErrUnverified
	}
	pathInfo, err := os.Lstat(s.dir)
	if err != nil || !os.SameFile(info, pathInfo) {
		release()
		return nil, ErrUnverified
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		release()
		return nil, err
	}
	claims, err := loadPending(root)
	if err != nil {
		root.Close()
		release()
		return nil, err
	}
	current, found := claims[reservationKey(b)]
	if !found || current != b {
		root.Close()
		release()
		return nil, ErrUnverified
	}
	return &PendingGuard{fd: fd, root: root, store: s, binding: b}, nil
}

func (g *PendingGuard) Close() error {
	if g == nil || g.fd == nil {
		return ErrUnverified
	}
	err := syscall.Flock(int(g.fd.Fd()), syscall.LOCK_UN)
	g.root.Close()
	g.fd.Close()
	g.fd = nil
	g.root = nil
	return err
}
