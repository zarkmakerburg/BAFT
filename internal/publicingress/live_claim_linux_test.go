//go:build linux

package publicingress

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A successful bind is not an exclusive TCP-port claim. Linux permits
// tcp4/tcp6 and distinct loopback-address listeners with the same port.
// The forwarded endpoint must never enter service while another listener
// already owns that numeric port, regardless of its address or family.
func TestServeRejectsOtherActiveOwnerOfNumericPortLinux(t *testing.T) {
	for _, tc := range []struct {
		name, foreignNetwork, foreignIP, candidateNetwork, candidateIP string
	}{
		{"v4-existing-v6-candidate", "tcp4", "127.0.0.1", "tcp6", "::1"},
		{"v6-existing-v4-candidate", "tcp6", "::1", "tcp4", "127.0.0.1"},
		{"other-v4-interface", "tcp4", "127.0.0.1", "tcp4", "127.0.0.2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			foreign, err := net.Listen(tc.foreignNetwork, net.JoinHostPort(tc.foreignIP, "0"))
			if err != nil {
				t.Skipf("foreign family unavailable: %v", err)
			}
			defer foreign.Close()
			port := foreign.Addr().(*net.TCPAddr).Port
			candidate, err := net.Listen(tc.candidateNetwork, net.JoinHostPort(tc.candidateIP, strconv.Itoa(port)))
			if err != nil {
				t.Skipf("kernel already forbids collision: %v", err)
			}
			defer candidate.Close()
			target := echoBackend(t)
			f, err := New(Config{Listen: candidate.Addr().String(), Target: target.Addr().String()})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			defer cancel()
			if err = f.Serve(ctx, candidate); err == nil {
				t.Fatal("unrelated existing TCP listener was not protected (Serve incorrectly succeeded)")
			} else if !strings.Contains(err.Error(), "already held by another live TCP listener") {
				t.Fatalf("admission failed for the wrong reason, not verified live ownership: %v", err)
			}
			// Admission failure must leave both caller-owned/foreign sockets
			// untouched; no failed bind may take over or interrupt either.
			for _, ln := range []net.Listener{foreign, candidate} {
				probe, err := net.DialTimeout("tcp", ln.Addr().String(), time.Second)
				if err != nil {
					t.Fatalf("rejected Serve closed a listener: %v", err)
				}
				_ = ln.(*net.TCPListener).SetDeadline(time.Now().Add(time.Second))
				accepted, err := ln.Accept()
				if err != nil {
					t.Fatalf("rejected Serve changed existing listener: %v", err)
				}
				accepted.Close()
				probe.Close()
			}
		})
	}
}

func TestServeAcceptsExclusiveNumericPortLinux(t *testing.T) {
	target := echoBackend(t)
	ln := tcpListener(t)
	f, err := New(Config{Listen: ln.Addr().String(), Target: target.Addr().String()})
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyNoOtherLiveTCPListeners(ln, ln.Addr().(*net.TCPAddr).Port); err != nil {
		t.Fatalf("exclusive existing listener rejected: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- f.Serve(ctx, ln) }()
	client := connect(t, ln.Addr().String())
	_ = client.SetDeadline(time.Now().Add(time.Second))
	if _, err := client.Write([]byte{0, 255, 1, 2}); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 4)
	if _, err := io.ReadFull(client, got); err != nil {
		t.Fatal(err)
	}
	if got[0] != 0 || got[1] != 255 || got[2] != 1 || got[3] != 2 {
		t.Fatalf("binary stream changed: %v", got)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("forwarder failed to shut down")
	}
}

// A parser error must never be misreported as successful collision detection,
// and listener inode identity must distinguish the caller from other sockets.
func TestInspectSocketTableStrictLinux(t *testing.T) {
	const port = 4242
	const own = uint64(1729)
	row := func(inode uint64) string {
		return fmt.Sprintf("0: 0100007F:1092 00000000:0000 0A 00000000:00000000 00:00000000 00000000 0 0 %d 1\n", inode)
	}
	for _, header := range []string{
		"sl local_address rem_address st\n",
		"sl local_address remote_address st\n",
	} {
		raw := []byte(header + row(own))
		got, err := inspectSocketTable(raw, false, port, own)
		if err != nil || !got.own || got.other {
			t.Fatalf("own socket rejected: %+v %v", got, err)
		}
		got, err = inspectSocketTable([]byte(header+row(own)+row(1850)), false, port, own)
		if err != nil || !got.own || !got.other {
			t.Fatalf("foreign socket missed: %+v %v", got, err)
		}
	}
	for _, raw := range []string{
		"",
		"sl local_address rem_address st",
		"sl local_address garbage st\n",
		"sl local_address rem_address st\n0: garbage\n",
		"sl local_address rem_address st\n0: 0000000Z:1092 00000000:0000 0A 00000000:00000000 00:00000000 00000000 0 0 1729 1\n",
	} {
		if _, err := inspectSocketTable([]byte(raw), false, port, own); err == nil {
			t.Fatalf("malformed inventory accepted: %q", raw)
		}
	}
}
