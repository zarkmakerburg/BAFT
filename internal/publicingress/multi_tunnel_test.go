package publicingress

import (
	"bytes"
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
