//go:build linux

package publicingress

import (
	"context"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A listener can acquire its original port exclusively and later become
// unsafe because another process binds the same numeric port on another IP
// or family. Detection must stop new ingress while leaving the other owner
// untouched; startup-only checks are insufficient.
func TestForwarderStopsOnLateCrossFamilyPortOwnerLinux(t *testing.T) {
	for _, tc := range []struct {
		name, network, listenIP, foreignNetwork, foreignIP string
	}{
		{"ipv4_then_ipv6", "tcp4", "127.0.0.1", "tcp6", "::1"},
		{"ipv6_then_ipv4", "tcp6", "::1", "tcp4", "127.0.0.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := echoBackend(t)
			ln, err := net.Listen(tc.network, net.JoinHostPort(tc.listenIP, "0"))
			if err != nil {
				t.Skipf("family unavailable: %v", err)
			}
			defer ln.Close()
			f, err := New(Config{Listen: ln.Addr().String(), Target: backend.Addr().String()})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- f.Serve(ctx, ln) }()
			client, err := net.DialTimeout(tc.network, ln.Addr().String(), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			_ = client.SetDeadline(time.Now().Add(time.Second))
			if _, err := client.Write([]byte{0x00, 0xff}); err != nil {
				t.Fatal(err)
			}
			echo := make([]byte, 2)
			if _, err := io.ReadFull(client, echo); err != nil {
				t.Fatalf("initial ingress unhealthy: %v", err)
			}
			if echo[0] != 0 || echo[1] != 255 {
				t.Fatalf("initial binary payload corrupted: %v", echo)
			}

			port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
			foreign, err := net.Listen(tc.foreignNetwork, net.JoinHostPort(tc.foreignIP, port))
			if err != nil {
				t.Skipf("kernel disallows independent same-port family: %v", err)
			}
			defer foreign.Close()
			select {
			case err := <-done:
				if err == nil || !strings.Contains(err.Error(), "already held by another live TCP listener") {
					t.Fatalf("watch did not report the foreign owner: %v", err)
				}
			case <-time.After(4 * time.Second):
				t.Fatal("late foreign owner was not detected; ingress remained active")
			}
			if f.ActiveConnections() != 0 {
				t.Fatal("late conflict left admitted clients active")
			}
			// The watcher is never allowed to close a foreign service.
			probe, err := net.DialTimeout(tc.foreignNetwork, foreign.Addr().String(), time.Second)
			if err != nil {
				t.Fatalf("foreign service was interrupted: %v", err)
			}
			defer probe.Close()
			_ = foreign.(*net.TCPListener).SetDeadline(time.Now().Add(time.Second))
			accepted, err := foreign.Accept()
			if err != nil {
				t.Fatalf("foreign service stopped accepting: %v", err)
			}
			_ = accepted.Close()
		})
	}
}
