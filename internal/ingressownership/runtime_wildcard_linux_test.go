//go:build linux

package ingressownership

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// ownedListenerKernelClosureCheck records the original socket's *inode*, not
// just its numeric port. /proc may contain a foreign LISTEN/TIME_WAIT socket
// on the same port after retirement; those do not imply BAFT leaked an FD.
func ownedListenerKernelClosureCheck(t *testing.T, ln *net.TCPListener) func() {
	t.Helper()
	f, err := ln.File()
	if err != nil {
		t.Fatalf("cannot inspect owned Linux listener inode: %v", err)
	}
	fi, err := f.Stat()
	_ = f.Close()
	if err != nil {
		t.Fatalf("cannot stat owned Linux listener inode: %v", err)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || st.Ino == 0 {
		t.Fatalf("unavailable original Linux socket inode")
	}
	inode := strconv.FormatUint(st.Ino, 10)
	return func() {
		t.Helper()
		for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("socket inode closure cannot be proven (%s): %v", path, err)
			}
			for _, line := range strings.Split(string(raw), "\n") {
				cols := strings.Fields(line)
				if len(cols) > 9 && cols[9] == inode {
					t.Fatalf("retired wildcard listener socket inode %s remains in %s: state=%s line=%s", inode, path, cols[3], line)
				}
			}
		}
		// An open local duplicate may retain the listener even if an OS
		// socket table snapshot unexpectedly omits the matching endpoint.
		descriptors, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Fatalf("cannot inspect current process socket FDs: %v", err)
		}
		for _, fd := range descriptors {
			link, err := os.Readlink("/proc/self/fd/" + fd.Name())
			if err == nil && link == fmt.Sprintf("socket:[%s]", inode) {
				t.Fatalf("retired wildcard listener inode %s still open as local FD %s", inode, fd.Name())
			}
		}
	}
}

// This one exact kernel counterexample demonstrates why rebinding a port
// after verified retirement is not a valid listener-leak test oracle.
func TestRuntimeWildcardForeignRebindAfterRetireDoesNotImplyLeakLinux(t *testing.T) {
	spec := testSpec(t, "wildcard-reclaim")
	host, port, err := net.SplitHostPort(spec.Config.Listen)
	if err != nil || host == "" {
		t.Fatal(err)
	}
	spec.Config.Listen = net.JoinHostPort("0.0.0.0", port)
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRuntime()
	if err := r.Apply(context.Background(), spec, proof); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	original := r.sessions[spec.Config.Listen].listener.(*net.TCPListener)
	r.mu.Unlock()
	proveClosed := ownedListenerKernelClosureCheck(t, original)
	if err := r.Retire(spec.Config.Listen, proof); err != nil {
		t.Fatal(err)
	}
	proveClosed()
	foreign, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.2", port))
	if err != nil {
		t.Skipf("kernel lacks alternate Linux loopback service address: %v", err)
	}
	defer foreign.Close()
	// The old wildcard rebind assertion would incorrectly say "retire
	// leaked socket" here, despite the inode proof above.
	rebound, err := listenWithForensics(t, "tcp4", spec.Config.Listen)
	if err == nil {
		rebound.Close()
		t.Fatal("kernel unexpectedly permitted wildcard LISTEN across foreign live same-port service")
	}
	if !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("unexpected wildcard rebind conflict: %v", err)
	}
	// The unrelated owner's inode remains alive; never close it to fake
	// success or conflate its lifetime with the BAFT listener.
	client, err := net.Dial("tcp4", foreign.Addr().String())
	if err != nil {
		t.Fatalf("foreign owner was destroyed by BAFT Retire: %v", err)
	}
	_ = client.Close()
}
