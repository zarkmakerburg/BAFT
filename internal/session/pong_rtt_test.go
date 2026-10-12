package session

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/protocol"
)

func pongPeer(t *testing.T, samples *[]time.Duration) *Peer {
	t.Helper()
	var out bytes.Buffer
	p, err := New(Dialer, Carrier{In: bytes.NewReader(nil), Out: &out}, "urn:baft:node:ex-01", nil, Options{
		NodeID: "ir-01", ExpectedPeerNodeID: "ex-01",
		LatencyObserver: func(rtt time.Duration) { *samples = append(*samples, rtt) },
	})
	if err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	p.localReady, p.peerReady = true, true
	p.markReadyLocked()
	p.mu.Unlock()
	return p
}

func pong(stamp uint64) protocol.Frame {
	payload := make([]byte, 8)
	binary.BigEndian.PutUint64(payload, stamp)
	return protocol.Frame{Type: protocol.TypePong, Payload: payload}
}

// A PONG that arrives more than a minute after its PING (it waited behind a
// stalled carrier) used to end the Session with "implausible PONG RTT", which
// took the whole agent down. It must only be dropped as an RTT sample.
func TestLatePongIsDroppedNotFatal(t *testing.T) {
	var samples []time.Duration
	p := pongPeer(t, &samples)
	p.pingBase = time.Now().Add(-2 * maxPlausiblePongRTT)
	if err := p.handleFrame(context.Background(), pong(1)); err != nil {
		t.Fatalf("late PONG ended the session: %v", err)
	}
	if len(samples) != 0 {
		t.Fatalf("late PONG reported as RTT %v", samples)
	}
}

// Stamps this Peer never sent (zero, from the future, or a wall-clock stamp
// echoed from an older build) are dropped too.
func TestForeignPongStampsAreDroppedNotFatal(t *testing.T) {
	var samples []time.Duration
	p := pongPeer(t, &samples)
	for _, stamp := range []uint64{0, p.pingStamp() + uint64(time.Hour), uint64(time.Now().UnixNano())} {
		if err := p.handleFrame(context.Background(), pong(stamp)); err != nil {
			t.Fatalf("PONG stamp %d ended the session: %v", stamp, err)
		}
	}
	if len(samples) != 0 {
		t.Fatalf("foreign stamps reported as RTT %v", samples)
	}
}

func TestPongReportsMonotonicRTT(t *testing.T) {
	var samples []time.Duration
	p := pongPeer(t, &samples)
	sent := p.pingStamp()
	time.Sleep(5 * time.Millisecond)
	if err := p.handleFrame(context.Background(), pong(sent)); err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 || samples[0] < 5*time.Millisecond || samples[0] > maxPlausiblePongRTT {
		t.Fatalf("RTT samples %v", samples)
	}
}

func TestMalformedPongIsStillAProtocolError(t *testing.T) {
	var samples []time.Duration
	p := pongPeer(t, &samples)
	if err := p.handlePong(protocol.Frame{Type: protocol.TypePong, Payload: []byte{1, 2, 3}}); err == nil {
		t.Fatal("a 3-byte PONG was accepted")
	}
}
