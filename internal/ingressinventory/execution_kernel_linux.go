//go:build linux

package ingressinventory

import (
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
)

const maxProcessMetadata = 16 << 10

func readBoundedKernelFile(ctx context.Context, path string) ([]byte, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, ErrCoverageIncomplete
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.Join(ErrCoverageIncomplete, err)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxProcessMetadata+1))
	if err != nil || len(b) == 0 || len(b) > maxProcessMetadata || ctx.Err() != nil {
		return nil, errors.Join(ErrCoverageIncomplete, err, ctx.Err())
	}
	return b, nil
}

// Kernel field #22 = process start ticks. Parenthesized comm may contain
// whitespace and literal ')' characters; never split the entire stat line.
func parseKernelProcessStart(raw []byte, actualPID int) (uint64, error) {
	s := string(raw)
	open := strings.IndexByte(s, '(')
	close := strings.LastIndexByte(s, ')')
	if open < 1 || close <= open || close+1 >= len(s) {
		return 0, ErrCoverageIncomplete
	}
	n, err := strconv.Atoi(strings.TrimSpace(s[:open]))
	if err != nil || n != actualPID || n < 1 {
		return 0, ErrCoverageIncomplete
	}
	fields := strings.Fields(s[close+1:])
	if len(fields) < 20 || len(fields[0]) != 1 {
		return 0, ErrCoverageIncomplete
	}
	ticks, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil || ticks == 0 {
		return 0, ErrCoverageIncomplete
	}
	return ticks, nil
}

func observeLinuxExecution(ctx context.Context, e PrivateLABExecutionEvidence) (PrivateLABExecutionEvidence, error) {
	var zero PrivateLABExecutionEvidence
	raw, err := readBoundedKernelFile(ctx, "/proc/self/stat")
	if err != nil {
		return zero, err
	}
	ticks, err := parseKernelProcessStart(raw, os.Getpid())
	if err != nil {
		return zero, err
	}
	dev, inode, digest, err := digestRunningExecutable(ctx)
	if err != nil {
		return zero, err
	}
	mount, err := os.Readlink("/proc/self/ns/mnt")
	if err != nil || !strings.HasPrefix(mount, "mnt:[") || !strings.HasSuffix(mount, "]") {
		return zero, ErrCoverageIncomplete
	}
	cgroupNS, err := os.Readlink("/proc/self/ns/cgroup")
	if err != nil || !strings.HasPrefix(cgroupNS, "cgroup:[") || !strings.HasSuffix(cgroupNS, "]") {
		return zero, ErrCoverageIncomplete
	}
	cgroup, err := readBoundedKernelFile(ctx, "/proc/self/cgroup")
	if err != nil {
		return zero, err
	}
	if ctx.Err() != nil {
		return zero, errors.Join(ErrCoverageIncomplete, ctx.Err())
	}
	e.ProcessStartTicks, e.ExecutableDevice, e.ExecutableInode = ticks, dev, inode
	e.ExecutableSHA256 = digest
	e.MountNS, e.CgroupNS, e.CgroupMembership = mount, cgroupNS, string(cgroup)
	return e, nil
}
