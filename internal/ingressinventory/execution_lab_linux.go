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

// measureManagedExecution measures the execution scope the installed policy
// declares: the isolated loopback-only singleton LAB (profile "") or the
// ADR-003 host profile. The host profile drops only the two properties an
// ordinary server can never have (non-init network namespace, loopback-only
// singleton); every host-completeness requirement stays.
func measureManagedExecution(ctx context.Context, expectedNetNS, profile string) (PrivateLABExecutionEvidence, error) {
	switch profile {
	case "":
		return measurePrivateLinuxLAB(ctx, expectedNetNS)
	case ExecutionProfileHostV1:
		return measureHostExecution(ctx, expectedNetNS)
	}
	return PrivateLABExecutionEvidence{}, fmt.Errorf("%w: unknown execution profile", ErrCoverageIncomplete)
}

func measureHostExecution(ctx context.Context, expectedNetNS string) (PrivateLABExecutionEvidence, error) {
	var e PrivateLABExecutionEvidence
	if ctx == nil || ctx.Err() != nil {
		return e, ErrCoverageIncomplete
	}
	current, err := namespaceIdentity()
	if err != nil || current != expectedNetNS {
		return e, fmt.Errorf("%w: changed Linux network namespace", ErrCoverageIncomplete)
	}
	if err := checkHostComplete(); err != nil {
		return e, err
	}
	initNS, err := os.Readlink("/proc/1/ns/net")
	if err != nil || !strings.HasPrefix(initNS, "net:[") {
		return e, fmt.Errorf("%w: init network namespace unreadable", ErrCoverageIncomplete)
	}
	return PrivateLABExecutionEvidence{CurrentNetNS: current, InitNetNS: initNS, OwnPID: os.Getpid()}, nil
}

func measurePrivateLinuxLAB(ctx context.Context, expectedNetNS string) (PrivateLABExecutionEvidence, error) {
	var e PrivateLABExecutionEvidence
	if ctx == nil || ctx.Err() != nil {
		return e, ErrCoverageIncomplete
	}
	current, err := namespaceIdentity()
	if err != nil || current != expectedNetNS {
		return e, fmt.Errorf("%w: changed Linux network namespace", ErrCoverageIncomplete)
	}
	initNS, iface, err := checkLinuxLABScope(current)
	if err != nil {
		return e, err
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return e, errors.Join(ErrCoverageIncomplete, err)
	}
	return measureLinuxLABProcesses(ctx, e, current, initNS, iface, entries)
}

// CheckManagedExecutionScope reports whether this process runs where the
// installed execution profile lets operational public ingress Apply run:
// profile "" needs the privileged, host-complete, isolated loopback-only LAB
// namespace; ExecutionProfileHostV1 needs a privileged, host-complete process
// on an ordinary server. These are static properties of where the agent runs;
// an agent that fails them must not advertise operational ingress, otherwise
// BCC queues public ingress that every Apply refuses.
func CheckManagedExecutionScope(ctx context.Context, profile string) error {
	if ctx == nil || ctx.Err() != nil {
		return ErrCoverageIncomplete
	}
	current, err := namespaceIdentity()
	if err != nil {
		return fmt.Errorf("%w: unknown Linux network namespace", ErrCoverageIncomplete)
	}
	switch profile {
	case "":
		_, _, err = checkLinuxLABScope(current)
		return err
	case ExecutionProfileHostV1:
		return checkHostComplete()
	}
	return fmt.Errorf("%w: unknown execution profile", ErrCoverageIncomplete)
}

// checkHostComplete: privileged root in the host-init PID and user
// namespaces with real procfs and systemd PID 1, so the process and kernel
// socket inventory is host-complete.
func checkHostComplete() error {
	// PID-private/container /proc can hide other processes sharing this
	// netns; a singleton observed through that partial table is NOT proof
	// of exclusive execution. The first supported LAB requires privileged
	// root in the host-init PID+user namespaces with real procfs/systemd PID1.
	if os.Geteuid() != 0 {
		return fmt.Errorf("%w: root host inventory required", ErrCoverageIncomplete)
	}
	var procfs unix.Statfs_t
	if err := unix.Statfs("/proc", &procfs); err != nil || procfs.Type != unix.PROC_SUPER_MAGIC {
		return fmt.Errorf("%w: untrusted procfs", ErrCoverageIncomplete)
	}
	initComm, err := os.ReadFile("/proc/1/comm")
	if err != nil || strings.TrimSpace(string(initComm)) != "systemd" {
		return fmt.Errorf("%w: non-systemd host init", ErrCoverageIncomplete)
	}
	for _, kind := range []string{"pid", "user"} {
		a, aErr := os.Readlink("/proc/self/ns/" + kind)
		b, bErr := os.Readlink("/proc/1/ns/" + kind)
		if aErr != nil || bErr != nil || a != b {
			return fmt.Errorf("%w: process inventory is not host-complete (%s namespace)", ErrCoverageIncomplete, kind)
		}
	}
	return nil
}

// checkLinuxLABScope verifies the static LAB scope and returns the init
// network namespace and the single interface name.
func checkLinuxLABScope(current string) (initNS, iface string, err error) {
	if err := checkHostComplete(); err != nil {
		return "", "", err
	}
	initNS, err = os.Readlink("/proc/1/ns/net")
	if err != nil || initNS == current || !strings.HasPrefix(initNS, "net:[") {
		return "", "", fmt.Errorf("%w: not an isolated non-init network namespace", ErrCoverageIncomplete)
	}
	interfaces, err := net.Interfaces()
	if err != nil || len(interfaces) != 1 || interfaces[0].Name != "lo" ||
		interfaces[0].Flags&net.FlagLoopback == 0 || interfaces[0].Flags&net.FlagUp == 0 {
		return "", "", fmt.Errorf("%w: managed LAB requires loopback-only network namespace", ErrCoverageIncomplete)
	}
	return initNS, interfaces[0].Name, nil
}

// measureLinuxLABProcesses walks /proc for processes sharing the managed
// network namespace (the dynamic part of the LAB execution evidence).
func measureLinuxLABProcesses(ctx context.Context, e PrivateLABExecutionEvidence, current, initNS, iface string, entries []os.DirEntry) (PrivateLABExecutionEvidence, error) {
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
		Interfaces: []string{iface}, Processes: shared,
	}
	return e, nil
}
