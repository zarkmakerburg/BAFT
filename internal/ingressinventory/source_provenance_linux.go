//go:build linux

package ingressinventory

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// protectedSourceRootIdentity independently pins each ancestor and the
// selected service-root directory. Root ownership is essential: a private
// non-root-owned directory can have safe-looking 0700 mode while its owner
// can replace stopped-unit claims between observation and mutation.
func protectedSourceRootIdentity(source string) (uint64, uint64, error) {
	components, err := pathComponents(source)
	if err != nil || len(components) == 0 {
		return 0, 0, ErrCoverageIncomplete
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return 0, 0, errors.Join(ErrCoverageIncomplete, err)
	}
	// Traversal replaces fd; close the final owned descriptor, not the
	// initial descriptor number which may already have been reused.
	defer func() { _ = unix.Close(fd) }()
	var st unix.Stat_t
	for _, component := range components {
		next, err := unix.Openat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if err != nil {
			return 0, 0, errors.Join(ErrCoverageIncomplete, err)
		}
		unix.Close(fd)
		fd = next
		if err := unix.Fstat(fd, &st); err != nil || st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Uid != 0 || st.Mode&0022 != 0 {
			return 0, 0, ErrCoverageIncomplete
		}
	}
	return uint64(st.Dev), st.Ino, nil
}

// scanPinnedSourceFiles records relevant drop-in directories (even empty),
// hashes actual unit/override bytes and rejects
// non-root-owned, symlinked, modified or writable source content.
// It is a read-only second independent observation AFTER the socket scanner
// has parsed effective stopped-service port claims.
func scanPinnedSourceFiles(directory string) ([]SourceFileIdentity, map[string]sourceObjectIdentity, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, nil, ErrCoverageIncomplete
	}
	defer root.Close()
	openEntries := func(relative string) ([]os.DirEntry, error) {
		f, err := root.Open(relative)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return f.ReadDir(-1)
	}
	entries, err := openEntries(".")
	if err != nil || len(entries) > 2048 {
		return nil, nil, ErrCoverageIncomplete
	}
	directories := make(map[string]sourceObjectIdentity)
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".socket") {
			names = append(names, e.Name())
		}
		if strings.HasSuffix(e.Name(), ".socket.d") {
			sub := e.Name()
			info, err := root.Lstat(sub)
			if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0022 != 0 {
				return nil, nil, ErrCoverageIncomplete
			}
			st, ok := info.Sys().(*syscall.Stat_t)
			if !ok || st.Uid != 0 {
				return nil, nil, ErrCoverageIncomplete
			}
			identity, err := stableSourceIdentity(info)
			if err != nil {
				return nil, nil, ErrCoverageIncomplete
			}
			directories[sub] = identity
			children, err := openEntries(sub)
			if err != nil || len(children) > 128 {
				return nil, nil, ErrCoverageIncomplete
			}
			for _, file := range children {
				if strings.HasSuffix(file.Name(), ".conf") {
					names = append(names, path.Join(sub, file.Name()))
				}
			}
		}
	}
	sort.Strings(names)
	if len(names) > 2048 {
		return nil, nil, ErrCoverageIncomplete
	}
	var observed []SourceFileIdentity
	for _, relative := range names {
		fi, err := root.Lstat(relative)
		if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0022 != 0 || fi.Size() < 0 || fi.Size() > 64<<10 {
			return nil, nil, ErrCoverageIncomplete
		}
		st, ok := fi.Sys().(*syscall.Stat_t)
		if !ok || st.Uid != 0 {
			return nil, nil, ErrCoverageIncomplete
		}
		f, err := root.Open(relative)
		if err != nil {
			return nil, nil, ErrCoverageIncomplete
		}
		opened, err := f.Stat()
		if err != nil || !os.SameFile(fi, opened) {
			_ = f.Close()
			return nil, nil, ErrCoverageIncomplete
		}
		raw, err := io.ReadAll(io.LimitReader(f, 64<<10+1))
		_ = f.Close()
		if err != nil || len(raw) > 64<<10 {
			return nil, nil, ErrCoverageIncomplete
		}
		newfi, err := root.Lstat(relative)
		if err != nil || !os.SameFile(fi, newfi) || !fi.ModTime().Equal(newfi.ModTime()) || fi.Size() != newfi.Size() {
			return nil, nil, ErrCoverageIncomplete
		}
		sum := sha256.Sum256(raw)
		observed = append(observed, SourceFileIdentity{Path: relative, SHA256: hex.EncodeToString(sum[:]), Device: uint64(st.Dev), Inode: st.Ino, ModUnixNS: fi.ModTime().UnixNano()})
	}
	return observed, directories, nil
}
