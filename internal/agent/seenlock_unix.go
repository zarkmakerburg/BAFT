//go:build unix

package agent

import (
	"errors"
	"fmt"
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
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
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
	if info.Size() > maxSeenJournalBytes {
		return nil, errors.New("seen-job replay journal exceeds size limit")
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxSeenJournalBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxSeenJournalBytes {
		return nil, errors.New("seen-job replay journal exceeds size limit")
	}
	return raw, nil
}

// validateSeenStateDir requires a real owned directory that other users
// cannot modify before any signed job consults its persistent replay fence.
// Private ledger and lock files remain mode 0600 even if the directory is
// read/searchable by others. MkdirAll never repairs insecure permissions.
func validateSeenStateDir(path string) error {
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !pathInfo.IsDir() || pathInfo.Mode().Perm()&0700 != 0700 || pathInfo.Mode().Perm()&0022 != 0 || pathInfo.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return fmt.Errorf("seen-job state directory must be private: mode=%v", pathInfo.Mode())
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
	if !os.SameFile(pathInfo, info) || !info.IsDir() || info.Mode().Perm()&0700 != 0700 || info.Mode().Perm()&0022 != 0 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || !ok || int(stat.Uid) != os.Geteuid() {
		return errors.New("seen-job state directory inode, permissions or owner invalid")
	}
	return nil
}
