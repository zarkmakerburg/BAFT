//go:build linux

package ingressownership

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func TestRetireRebindDiagnosticSeparatesOriginalFromForeignLinux(t *testing.T) {
	original, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer original.Close()
	address := original.Addr().String()
	diagnostic := captureRetireRebindDiagnostic(t, original)
	if live := diagnostic(); !strings.Contains(live, "original=true") {
		t.Fatalf("missing live original inode: %s", live)
	}
	if err := original.Close(); err != nil {
		t.Fatal(err)
	}
	foreign, err := net.Listen("tcp4", address)
	if err != nil {
		t.Fatal(err)
	}
	defer foreign.Close()
	candidate, err := net.Listen("tcp4", address)
	if err == nil {
		candidate.Close()
		t.Fatal("kernel allowed duplicate foreign bind")
	}
	if !errors.Is(err, syscall.EADDRINUSE) {
		t.Fatalf("expected actual EADDRINUSE: %v", err)
	}
	snapshot := diagnostic()
	if strings.Contains(snapshot, "original=true") || !strings.Contains(snapshot, "state=0A") || !strings.Contains(snapshot, "original=false") || !strings.Contains(snapshot, "original_inode_local_fd_count=0") {
		t.Fatalf("failed to distinguish foreign live socket from closed original: %s", snapshot)
	}
}

// Capture identity while the owned socket is alive, then collect fresh bounded
// evidence only if rebind fails. This does not guess which process owns an
// unrelated socket or change any socket/firewall/retry behavior.
func captureRetireRebindDiagnostic(t *testing.T, ln *net.TCPListener) func() string {
	t.Helper()
	f, err := ln.File()
	if err != nil {
		t.Fatal(err)
	}
	info, err := f.Stat()
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Ino == 0 {
		t.Fatal("original socket inode unavailable")
	}
	inode := strconv.FormatUint(stat.Ino, 10)
	_, portText, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil {
		t.Fatal(err)
	}
	return func() string {
		var result strings.Builder
		fmt.Fprintf(&result, "original_inode=%s endpoint=%s\n", inode, ln.Addr())
		for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
			raw, err := os.ReadFile(path)
			if err != nil {
				fmt.Fprintf(&result, "%s: %v\n", path, err)
				continue
			}
			matched := 0
			for _, line := range strings.Split(string(raw), "\n") {
				cols := strings.Fields(line)
				if len(cols) < 10 {
					continue
				}
				endpoint := strings.Split(cols[1], ":")
				if len(endpoint) != 2 {
					continue
				}
				p, err := strconv.ParseUint(endpoint[1], 16, 16)
				if err == nil && (p == port || cols[9] == inode) {
					if matched < 32 {
						fmt.Fprintf(&result, "%s state=%s inode=%s original=%t row=%s\n", path, cols[3], cols[9], cols[9] == inode, line)
					}
					matched++
				}
			}
			fmt.Fprintf(&result, "%s matching_rows=%d\n", path, matched)
		}
		fds, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			fmt.Fprintf(&result, "local_fds: %v\n", err)
			return result.String()
		}
		found := 0
		for _, fd := range fds {
			if link, err := os.Readlink("/proc/self/fd/" + fd.Name()); err == nil && link == "socket:["+inode+"]" {
				fmt.Fprintf(&result, "original_inode_local_fd=%s\n", fd.Name())
				found++
			}
		}
		fmt.Fprintf(&result, "original_inode_local_fd_count=%d", found)
		return result.String()
	}
}
