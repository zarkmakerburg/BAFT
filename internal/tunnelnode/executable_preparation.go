package tunnelnode

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// Shared installer binaries can change independently of instance config. Seal
// their actual bytes and DAC identity, not a claimed version string. This is
// a bounded drift check, not an atomic upgrade lock or executable attestation.
func executableIdentity(ctx context.Context, path string) (string, error) {
	if !filepath.IsAbs(path) || realDirectoryAncestors(filepath.Dir(path)) != nil {
		return "", errors.New("candidate executable requires an absolute path with real directories")
	}
	f, err := openNoFollow(path)
	if err != nil {
		return "", errors.New("candidate executable is missing, unreadable or a symlink")
	}
	defer f.Close()
	before, err := f.Stat()
	const maxExecutableBytes = 128 << 20
	if err != nil || !before.Mode().IsRegular() || before.Mode().Perm()&0111 == 0 || before.Size() < 1 || before.Size() > maxExecutableBytes {
		return "", errors.New("candidate executable is not a bounded executable regular file")
	}
	owner, ok := before.Sys().(*syscall.Stat_t)
	if !ok {
		return "", errors.New("candidate executable ownership is unavailable")
	}
	h := sha256.New()
	fmt.Fprintf(h, "%o:%d:%d\n", before.Mode(), owner.Uid, owner.Gid)
	buf := make([]byte, 64<<10)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, err := f.Read(buf)
		total += int64(n)
		if total > maxExecutableBytes {
			return "", errors.New("candidate executable exceeded its size limit")
		}
		h.Write(buf[:n])
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", errors.New("candidate executable read failed")
		}
	}
	after, err := f.Stat()
	if err != nil {
		return "", errors.New("candidate executable changed while reading")
	}
	current, err := os.Lstat(path)
	afterOwner, afterOK := after.Sys().(*syscall.Stat_t)
	if err != nil || !os.SameFile(before, current) || current.Mode() != before.Mode() ||
		before.Mode() != after.Mode() || total != before.Size() || before.Size() != after.Size() ||
		!before.ModTime().Equal(after.ModTime()) || !afterOK || owner.Uid != afterOwner.Uid || owner.Gid != afterOwner.Gid {
		return "", errors.New("candidate executable changed while reading")
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func (m *Manager) prepareExecutables(ctx context.Context, required bool) (map[string]string, error) {
	if !required {
		return nil, nil
	} // old protocol and persisted receipts
	out := make(map[string]string, 2)
	for _, path := range []string{m.BaftBin, m.PairBin} {
		identity, err := executableIdentity(ctx, path)
		if err != nil {
			return nil, err
		}
		out[path] = identity
	}
	return out, nil
}

func (m *Manager) verifyExecutables(ctx context.Context, t Txn) error {
	if len(t.PreparedExecutables) == 0 {
		return nil
	} // older in-flight receipt
	current, err := m.prepareExecutables(ctx, true)
	if err != nil {
		return err
	}
	if !sameDigests(current, t.PreparedExecutables) {
		return errors.New("candidate executable changed since preparation; explicit rollback and new preparation required")
	}
	return nil
}
