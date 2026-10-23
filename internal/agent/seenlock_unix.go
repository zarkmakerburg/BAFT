//go:build unix

package agent

import (
	"errors"
	"io"
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

// readSeenJournal refuses aliases and publicly accessible files before
// loading the replay-fence ledger, and verifies the opened inode still
// matches the path examined. A compromised ledger must never reset the
// admission history of signed jobs.
func readSeenJournal(path string) ([]byte, error) {
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !pathInfo.Mode().IsRegular() || pathInfo.Mode().Perm()&0077 != 0 {
		return nil, errors.New("seen-job journal must be a private regular file")
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !os.SameFile(pathInfo, info) || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || !ok || int(stat.Uid) != os.Geteuid() || stat.Nlink != 1 {
		return nil, errors.New("seen-job journal inode, permissions or ownership invalid")
	}
	return io.ReadAll(f)
}

// validateSeenStateDir requires a private owner-only directory before any
// signed job can consult or update its persistent replay fence. MkdirAll
// deliberately does not repair unsafe permissions on pre-existing paths.
func validateSeenStateDir(path string) error {
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !pathInfo.IsDir() || pathInfo.Mode().Perm() != 0700 || pathInfo.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return errors.New("seen-job state directory must be a private regular directory")
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !os.SameFile(pathInfo, info) || !info.IsDir() || info.Mode().Perm() != 0700 || !ok || int(stat.Uid) != os.Geteuid() {
		return errors.New("seen-job state directory inode, permissions or owner invalid")
	}
	return nil
}
