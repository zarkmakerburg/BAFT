//go:build unix

package agent

import (
	"errors"
	"os"
	"syscall"
)

// Claims are serialized across Agent processes as well as Agent objects.
// Nonblocking acquisition fails closed rather than executing while another
// writer holds the journal. A later poll may try an unclaimed job again.
func lockSeenJournal(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || !ok || int(stat.Uid) != os.Geteuid() || stat.Nlink != 1 {
		_ = f.Close()
		return nil, errors.New("seen-job lock must be a private, owned regular file")
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
