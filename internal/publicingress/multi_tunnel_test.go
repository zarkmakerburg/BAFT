package publicingress

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"
)

// Two independent tunnels must never share a listener or cancel each other's
// TCP sessions. Dedicated transport remains the fail-closed shared-mux fallback.
func TestIndependentTunnelLifecyclePreservesSiblingTraffic(t *testing.T) {
	targetA := echoBackend(t)
	targetB := echoBackend(t)
	first, addressA, stopA := launch(t, targetA.Addr().String(), 3)
	second, addressB, stopB := launch(t, targetB.Addr().String(), 3)
	if addressA == addressB {
		t.Fatal("independent listeners were assigned the same endpoint")
	}
	traffic := func(c net.Conn, b []byte) {
		t.Helper()
		if _, err := c.Write(b); err != nil {
			t.Fatal(err)
		}
		got := make([]byte, len(b))
		if _, err := io.ReadFull(c, got); err != nil || !bytes.Equal(b, got) {
			t.Fatalf("TCP stream corrupted or interrupted: got %d bytes, %v", len(got), err)
		}
	}
	clientA := connect(t, addressA)
	clientB := connect(t, addressB)
	traffic(clientA, bytes.Repeat([]byte("tunnel-A\x00"), 2000))
	traffic(clientB, bytes.Repeat([]byte("tunnel-B\xff"), 2000))

	// Simulate retiring only the first independently-owned tunnel.
	stopA()
	if first.ActiveConnections() != 0 {
		t.Fatal("retired tunnel left active client slots")
	}
	if second.ActiveConnections() != 1 {
		t.Fatalf("sibling session was lost: active=%d", second.ActiveConnections())
	}
	traffic(clientB, bytes.Repeat([]byte{0x00, 0x7f, 0xff, 0x80}, 16384))

	// The stopped listener is rebindable without causing a sibling restart.
	deadline := time.Now().Add(time.Second)
	for {
		replacement, err := net.Listen("tcp4", addressA)
		if err == nil {
			_ = replacement.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("retired listener not reclaimable: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	traffic(clientB, []byte("still-running-after-rebind"))
	stopB()
	if second.ActiveConnections() != 0 {
		t.Fatal("sibling shutdown leaked an active client")
	}
}

// IPv6 is an independently supported family, not an alias for IPv4. Verify
// actual end-to-end bytes through a native IPv6 loopback listener/backend.
func TestIPv6DedicatedIngressBinaryRoundtrip(t *testing.T) {
	backend, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	defer backend.Close()
	backendDone := make(chan struct{})
	go func() {
		defer close(backendDone)
		for {
			conn, err := backend.Accept()
			if err != nil {
				return
			}
			go func() { defer conn.Close(); _, _ = io.Copy(conn, conn) }()
		}
	}()
	ingress, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Fatalf("IPv6 listener: %v", err)
	}
	defer ingress.Close()
	f, err := New(Config{Listen: ingress.Addr().String(), Target: backend.Addr().String(), MaxClients: 2})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- f.Serve(ctx, ingress) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("IPv6 shutdown: %v", err)
		}
	}()
	client, err := net.DialTimeout("tcp6", ingress.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte{0, 1, 255, 128, 10}, 8192)
	if _, err := client.Write(payload); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(client, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("IPv6 ingress altered binary payload")
	}
}
