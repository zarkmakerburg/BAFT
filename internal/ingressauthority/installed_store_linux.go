//go:build linux

package ingressauthority

import (
	"context"
	"os"
	"syscall"

	"github.com/zarkmakerburg/baft/internal/ingressinventory"
	"golang.org/x/sys/unix"
)

const InstalledAuthorityDirectory = "/var/lib/baft-agent/ingress-authority"

// OpenInstalledManagedReservationStore uses one root-controlled resource store
// for all normal Agent operations. It never creates or replaces local state.
// A caller-chosen alternate reservation directory cannot mint a live permit.
func OpenInstalledManagedReservationStore(ctx context.Context, nodeID string) (*ReservationStore, error) {
	if _, err := ingressinventory.PinInstalledManagedLABExecution(ctx, nodeID); err != nil {
		return nil, err
	}
	s, err := OpenReservationStore(InstalledAuthorityDirectory)
	if err != nil {
		return nil, err
	}
	if err := checkInstalledAuthorityStore(ctx, s); err != nil {
		return nil, err
	}
	return s, nil
}

func checkInstalledAuthorityStore(ctx context.Context, s *ReservationStore) error {
	if ctx == nil || ctx.Err() != nil || s == nil || s.dir != InstalledAuthorityDirectory || os.Geteuid() != 0 {
		return ErrUnverified
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrUnverified
	}
	defer func() { _ = unix.Close(fd) }()
	for i, part := range []string{"", "var", "lib", "baft-agent", "ingress-authority"} {
		if part != "" {
			next, err := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err != nil {
				return ErrUnverified
			}
			_ = unix.Close(fd)
			fd = next
		}
		var st unix.Stat_t
		if unix.Fstat(fd, &st) != nil || st.Uid != 0 || st.Mode&0022 != 0 || st.Mode&07000 != 0 || i == 4 && st.Mode&0777 != 0700 {
			return ErrUnverified
		}
	}
	var final unix.Stat_t
	if unix.Fstat(fd, &final) != nil || s.identity == nil {
		return ErrUnverified
	}
	identity, ok := s.identity.Sys().(*syscall.Stat_t)
	if !ok || uint64(identity.Dev) != uint64(final.Dev) || identity.Ino != final.Ino {
		return ErrUnverified
	}
	return ctx.Err()
}
