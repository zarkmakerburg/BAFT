//go:build unix

package tunnelnode

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func lockLifecycleFile(ctx context.Context, path string, directory os.FileInfo) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return nil, err
	}
	refuse := func(err error) (func(), error) { _ = f.Close(); return nil, err }
	info, err := f.Stat()
	if err != nil {
		return refuse(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() != 0 || !ok || int(stat.Uid) != os.Geteuid() || stat.Nlink != 1 {
		return refuse(errors.New("lifecycle lock must be an empty private owned regular file with one link"))
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return refuse(fmt.Errorf("%w: %v", ErrLifecycleBusy, err))
		}
		return refuse(err)
	}
	current, err := os.Lstat(path)
	if err != nil {
		return refuse(err)
	}
	parent, err := os.Lstat(filepath.Dir(path))
	if err != nil {
		return refuse(err)
	}
	if !os.SameFile(info, current) || !os.SameFile(directory, parent) {
		return refuse(errors.New("lifecycle lock or directory identity changed"))
	}
	if err := ctx.Err(); err != nil {
		return refuse(err)
	}
	// Keep the inode permanently. Unlinking on unlock permits another process
	// to lock a different inode while an old holder still owns this one.
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
