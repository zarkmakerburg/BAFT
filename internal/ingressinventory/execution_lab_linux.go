//go:build linux

package ingressinventory

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

func measurePrivateLinuxLAB(ctx context.Context, expectedNetNS string) (PrivateLABExecutionEvidence, error) {
	var e PrivateLABExecutionEvidence
	if ctx == nil || ctx.Err() != nil {
		return e, ErrCoverageIncomplete
	}
	current, err := namespaceIdentity()
	if err != nil || current != expectedNetNS {
		return e, fmt.Errorf("%w: changed Linux network namespace", ErrCoverageIncomplete)
	}
	// PID-private/container /proc can hide other processes sharing this
	// netns; a singleton observed through that partial table is NOT proof
	// of exclusive execution. The first supported LAB requires privileged
	// root in the host-init PID+user namespaces with real procfs/systemd PID1.
	if os.Geteuid() != 0 {
		return e, fmt.Errorf("%w: root host inventory required", ErrCoverageIncomplete)
	}
	var procfs unix.Statfs_t
	if err := unix.Statfs("/proc", &procfs); err != nil || procfs.Type != unix.PROC_SUPER_MAGIC {
		return e, fmt.Errorf("%w: untrusted procfs", ErrCoverageIncomplete)
	}
	initComm, err := os.ReadFile("/proc/1/comm")
	if err != nil || strings.TrimSpace(string(initComm)) != "systemd" {
		return e, fmt.Errorf("%w: non-systemd host init", ErrCoverageIncomplete)
	}
	for _, kind := range []string{"pid", "user"} {
		a, aErr := os.Readlink("/proc/self/ns/" + kind)
		b, bErr := os.Readlink("/proc/1/ns/" + kind)
		if aErr != nil || bErr != nil || a != b {
			return e, fmt.Errorf("%w: process inventory is not host-complete (%s namespace)", ErrCoverageIncomplete, kind)
		}
	}
	initNS, err := os.Readlink("/proc/1/ns/net")
	if err != nil || initNS == current || !strings.HasPrefix(initNS, "net:[") {
		return e, fmt.Errorf("%w: not an isolated non-init network namespace", ErrCoverageIncomplete)
	}
	interfaces, err := net.Interfaces()
	if err != nil || len(interfaces) != 1 || interfaces[0].Name != "lo" ||
		interfaces[0].Flags&net.FlagLoopback == 0 || interfaces[0].Flags&net.FlagUp == 0 {
		return e, fmt.Errorf("%w: managed LAB requires loopback-only network namespace", ErrCoverageIncomplete)
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return e, errors.Join(ErrCoverageIncomplete, err)
	}
	pid := os.Getpid()
	var shared []int
	for _, entry := range entries {
		if ctx.Err() != nil {
			return e, errors.Join(ErrCoverageIncomplete, ctx.Err())
		}
		if !entry.IsDir() {
			continue
		}
		n, err := strconv.Atoi(entry.Name())
		if err != nil || n < 1 {
			continue
		}
		procDir := filepath.Join("/proc", entry.Name())
		nsPath := filepath.Join(procDir, "ns", "net")
		var ns string
		for attempt := 0; attempt < 4; attempt++ {
			ns, err = os.Readlink(nsPath)
			if err == nil {
				break
			}
			// /proc enumeration races with short-lived tasks. Retry a bounded
			// number of times, but only ignore a PID after procfs confirms it
			// has disappeared. A still-present unreadable PID remains UNKNOWN
			// and must fail closed; this is not a blanket permission fallback.
			_, statErr := os.Stat(procDir)
			if errors.Is(statErr, os.ErrNotExist) {
				err = nil
				break
			}
			if attempt < 3 {
				runtime.Gosched()
				time.Sleep(time.Millisecond)
			}
		}
		if err != nil {
			return e, fmt.Errorf("%w: cannot inspect live namespace holder PID %d", ErrCoverageIncomplete, n)
		}
		if ns == "" {
			// PID disappeared during the bounded retry above.
			continue
		}
		if ns == current {
			shared = append(shared, n)
		}
	}
	sort.Ints(shared)
	if len(shared) != 1 || shared[0] != pid {
		return e, fmt.Errorf("%w: concurrent process in the LAB network namespace: %v", ErrCoverageIncomplete, shared)
	}
	e = PrivateLABExecutionEvidence{
		CurrentNetNS: current, InitNetNS: initNS, OwnPID: pid,
		Interfaces: []string{interfaces[0].Name}, Processes: shared,
	}
	return e, nil
}
