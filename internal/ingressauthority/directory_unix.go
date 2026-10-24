//go:build unix

package ingressauthority

import (
	"os"
	"syscall"
)

// openPinnedAuthorityDir fails safely for FIFO, symlink, and non-directory
// objects instead of following attacker-controlled aliases or blocking.
func openPinnedAuthorityDir(path string, expected os.FileInfo) (*os.File, error) {
	raw, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrUnverified
	}
	fd := os.NewFile(uintptr(raw), path)
	info, err := fd.Stat()
	if err != nil {
		fd.Close()
		return nil, ErrUnverified
	}
	observed, err := os.Lstat(path)
	if err != nil || !os.SameFile(info, observed) ||
		expected != nil && !os.SameFile(info, expected) ||
		!validAuthorityDir(info) {
		fd.Close()
		return nil, ErrUnverified
	}
	return fd, nil
}

func validAuthorityDir(info os.FileInfo) bool {
	if info == nil || !info.IsDir() || info.Mode().Perm() != 0700 ||
		info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Geteuid()
}

// checkAuthorityRoot makes sure os.OpenRoot resolved the same exact locked
// inode. Verify it again under the held lock and after potential path swaps.
func checkAuthorityRoot(fd *os.File, root *os.Root, path string, expected os.FileInfo) error {
	if fd == nil || root == nil {
		return ErrUnverified
	}
	locked, err := fd.Stat()
	if err != nil || !validAuthorityDir(locked) ||
		expected != nil && !os.SameFile(locked, expected) {
		return ErrUnverified
	}
	anchored, err := root.Stat(".")
	if err != nil || !os.SameFile(locked, anchored) || !validAuthorityDir(anchored) {
		return ErrUnverified
	}
	named, err := os.Lstat(path)
	if err != nil || !os.SameFile(locked, named) {
		return ErrUnverified
	}
	return nil
}
