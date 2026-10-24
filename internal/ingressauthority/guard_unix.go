//go:build unix

package ingressauthority

import (
	"context"
	"errors"
	"os"
	"sync"
	"syscall"
)

var ErrGuardBusy = errors.New("ingress authority mutation lock busy")

// PendingGuard holds a private OS-level lock over a durable pending claim.
// It is not a LiveLease and never grants permission for host mutation.
type PendingGuard struct {
	mu      sync.Mutex
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
	fd, err := openPinnedAuthorityDir(s.dir, s.identity)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(fd.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		fd.Close()
		return nil, ErrGuardBusy
	}
	release := func() { _ = syscall.Flock(int(fd.Fd()), syscall.LOCK_UN); _ = fd.Close() }
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		release()
		return nil, err
	}
	if err := checkAuthorityRoot(fd, root, s.dir, s.identity); err != nil {
		root.Close()
		release()
		return nil, err
	}
	claims, err := loadPending(root)
	if err != nil {
		root.Close()
		release()
		return nil, err
	}
	old, ok := claims[reservationKey(b)]
	if !ok || old != b {
		root.Close()
		release()
		return nil, ErrUnverified
	}
	return &PendingGuard{fd: fd, root: root, store: s, binding: b}, nil
}

func (g *PendingGuard) CheckPending(ctx context.Context, b BindingV1) error {
	if g == nil {
		return ErrUnverified
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if ctx == nil || ctx.Err() != nil || g.fd == nil || g.root == nil || g.store == nil ||
		b != g.binding {
		return ErrUnverified
	}
	if err := checkAuthorityRoot(g.fd, g.root, g.store.dir, g.store.identity); err != nil {
		return err
	}
	claims, err := loadPending(g.root)
	if err != nil {
		return err
	}
	if current, ok := claims[reservationKey(b)]; !ok || current != b {
		return ErrUnverified
	}
	return nil
}

// Close serializes with CheckPending, and never deletes a durable claim.
func (g *PendingGuard) Close() error {
	if g == nil {
		return ErrUnverified
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.fd == nil {
		return ErrUnverified
	}
	err := syscall.Flock(int(g.fd.Fd()), syscall.LOCK_UN)
	if closeErr := g.root.Close(); err == nil {
		err = closeErr
	}
	if closeErr := g.fd.Close(); err == nil {
		err = closeErr
	}
	g.fd = nil
	g.root = nil
	return err
}
