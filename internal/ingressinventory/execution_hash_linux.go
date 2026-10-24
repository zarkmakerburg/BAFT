//go:build linux

package ingressinventory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"syscall"
)

// Only the kernel-selected running executable descriptor is trusted.
// Bound the file size and streamed hashing to 128 MiB; do not follow
// argv[0] or a caller-provided filesystem path.
func digestRunningExecutable(ctx context.Context) (uint64, uint64, string, error) {
	if ctx == nil || ctx.Err() != nil {
		return 0, 0, "", ErrCoverageIncomplete
	}
	f, err := os.Open("/proc/self/exe")
	if err != nil {
		return 0, 0, "", errors.Join(ErrCoverageIncomplete, err)
	}
	defer f.Close()
	before, err := f.Stat()
	const maxExecutable = 128 << 20
	if err != nil || !before.Mode().IsRegular() || before.Size() < 1 || before.Size() > maxExecutable {
		return 0, 0, "", ErrCoverageIncomplete
	}
	sys, ok := before.Sys().(*syscall.Stat_t)
	if !ok || sys.Ino == 0 {
		return 0, 0, "", ErrCoverageIncomplete
	}
	h := sha256.New()
	buffer := make([]byte, 32<<10)
	var total int64
	for {
		if ctx.Err() != nil {
			return 0, 0, "", errors.Join(ErrCoverageIncomplete, ctx.Err())
		}
		n, readErr := f.Read(buffer)
		total += int64(n)
		if total > maxExecutable {
			return 0, 0, "", ErrCoverageIncomplete
		}
		if n > 0 {
			_, _ = h.Write(buffer[:n])
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil || n == 0 {
			return 0, 0, "", errors.Join(ErrCoverageIncomplete, readErr)
		}
	}
	after, err := f.Stat()
	if err != nil || total != before.Size() || after.Size() != before.Size() ||
		!os.SameFile(before, after) {
		return 0, 0, "", ErrCoverageIncomplete
	}
	return uint64(sys.Dev), sys.Ino, hex.EncodeToString(h.Sum(nil)), nil
}
