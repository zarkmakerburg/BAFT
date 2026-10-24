//go:build unix

package bcc

import (
	"errors"
	"os"
	"syscall"
)

// openTrustedBackupKey fails closed on aliases, public permissions, foreign
// ownership or a replaced inode. Never follow the final path component.
func openTrustedBackupKey(path string) (*os.File, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Mode().Perm()&0077 != 0 || before.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return nil, errors.New("backup key file must be a private regular file")
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	actual, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	stat, ok := actual.Sys().(*syscall.Stat_t)
	if !os.SameFile(before, actual) || !actual.Mode().IsRegular() || actual.Mode().Perm()&0077 != 0 || !ok || stat.Nlink != 1 || int(stat.Uid) != os.Geteuid() {
		_ = f.Close()
		return nil, errors.New("backup key file inode, owner, permissions or link count invalid")
	}
	return f, nil
}
