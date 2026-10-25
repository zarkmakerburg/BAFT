package session

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/protocol"
	"github.com/zarkmakerburg/baft/internal/resources"
	"github.com/zarkmakerburg/baft/internal/routes"
)

// A Flow whose application connection stays silent in both directions for the
// configured window is torn down and releases its node flow slot, so an idle
// or half-dead peer cannot hold max_flows capacity forever.
func TestIdleFlowIsReapedAndReleasesNodeSlot(t *testing.T) {
	tbl, err := routes.New([]routes.Route{{ID: "main", Target: "127.0.0.1:2443", AllowedPeers: map[string]struct{}{"urn:baft:node:ir-01": {}}}})
	if err != nil {
		t.Fatal(err)
	}
	slots, err := resources.NewFlowSlots(1)
	if err != nil {
		t.Fatal(err)
	}
	out := &lockedBuffer{}
	p, err := New(Listener, Carrier{In: bytes.NewReader(nil), Out: out}, "urn:baft:node:ir-01", tbl, Options{
		NodeID: "ex-01", ExpectedPeerNodeID: "ir-01", ProfileID: "secure-fast", ProfileVersion: 1, ConfigRevision: "test",
		FlowSlots: slots, FlowIdleTimeout: 200 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !p.beginRunLifecycle() {
		t.Fatal("fixture could not model an active Session")
	}
	local, remote := tcpPair(t) // real TCP target: local is dialed by the Session, remote is the service
	p.dial = func(context.Context, string, string) (net.Conn, error) { return local, nil }
	req, _ := protocol.EncodeControl(protocol.OpenRequest{RouteID: "main", OpenNonce: "00112233445566778899aabbccddeeff"})
	if err := p.handleOpen(context.Background(), protocol.Frame{Type: protocol.TypeOpen, StreamID: 1, Payload: req}); err != nil {
		t.Fatal(err)
	}
	out.waitFrames(t, 2) // OPEN_OK + WINDOW
	if slots.Used() != 1 {
		t.Fatalf("open Flow holds %d slots, want 1", slots.Used())
	}
	p.mu.Lock()
	p.localReady, p.peerReady = true, true
	p.markReadyLocked()
	p.mu.Unlock()
	// The peer grants credit, which is what starts the Flow's local read pump.
	if err := p.handleFrame(context.Background(), protocol.Frame{Type: protocol.TypeWindow, StreamID: 1, Offset: 65536}); err != nil {
		t.Fatal(err)
	}

	// The service side sees the connection closed by the idle reaper.
	_ = remote.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.ReadAll(remote); err != nil && !isReset(err) {
		t.Fatalf("idle Flow was not closed: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for slots.Used() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("idle Flow still holds %d node slots", slots.Used())
		}
		time.Sleep(5 * time.Millisecond)
	}
	p.closeAll()
	p.wg.Wait()
}

func isReset(err error) bool {
	return err != nil && (err == io.ErrUnexpectedEOF || bytes.Contains([]byte(err.Error()), []byte("reset")))
}

// Real bytes in either direction extend the window; silence does not, and a
// disabled timeout (0) never wraps or closes anything.
func TestIdleConnActivityExtendsWindowAndZeroDisables(t *testing.T) {
	a, b := tcpPair(t)
	if got := newIdleConn(a, 0); got != a {
		t.Fatal("zero idle timeout must not wrap the connection")
	}
	w := newIdleConn(a, 250*time.Millisecond)
	// Activity every 100ms for 600ms (> idle window) keeps the connection open.
	stop := time.Now().Add(600 * time.Millisecond)
	for time.Now().Before(stop) {
		if _, err := b.Write([]byte("x")); err != nil {
			t.Fatalf("peer write: %v", err)
		}
		buf := make([]byte, 1)
		_ = w.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := w.Read(buf); err != nil {
			t.Fatalf("active connection was closed early: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	// Now silence: the wrapper closes the underlying conn within the window.
	_ = w.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := w.Read(make([]byte, 1)); err == nil {
		t.Fatal("silent connection was not closed")
	}
	// TCP-only teardown hooks still reach the real conn through the wrapper.
	if _, ok := w.(interface{ CloseWrite() error }); !ok {
		t.Fatal("wrapper hides CloseWrite")
	}
	if _, ok := w.(interface{ SetLinger(int) error }); !ok {
		t.Fatal("wrapper hides SetLinger")
	}
}
