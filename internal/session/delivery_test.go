package session

import (
	"bytes"
	"context"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/protocol"
)

func TestDeliveryObserverCountsSocketDeliveryNotAcceptedOrDuplicateData(t *testing.T) {
	var delivered atomic.Uint64
	out := &lockedBuffer{}
	p, err := New(Dialer, Carrier{In: bytes.NewReader(nil), Out: out}, "urn:baft:node:ex-01", nil, Options{NodeID: "ir-01", ExpectedPeerNodeID: "ex-01", DeliveryObserver: func(n uint64) { delivered.Add(n) }})
	if err != nil {
		t.Fatal(err)
	}
	if !p.beginRunLifecycle() {
		t.Fatal("no lifecycle")
	}
	local, target := net.Pipe()
	defer target.Close()
	f := newFlow(1, "main", "00112233445566778899aabbccddeeff", local, p.allocator)
	f.openOK = true
	p.flows[1] = f
	if _, err := p.reserveReceiveWindow(f); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer f.close()
	p.startTargetPump(ctx, f)
	payload := bytes.Repeat([]byte("delivery"), 256)
	frame := protocol.Frame{Type: protocol.TypeData, StreamID: 1, Payload: payload}
	if err := p.handleData(f, frame); err != nil {
		t.Fatal(err)
	}
	if delivered.Load() != 0 {
		t.Fatal("queued data was labeled delivered")
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(target, got); err != nil || !bytes.Equal(got, payload) {
		t.Fatal("application did not receive exact payload")
	}
	deadline := time.Now().Add(time.Second)
	for delivered.Load() != uint64(len(payload)) {
		if time.Now().After(deadline) {
			t.Fatal("successful socket delivery was not observed")
		}
		time.Sleep(time.Millisecond)
	}
	if err := p.handleData(f, frame); err != nil {
		t.Fatal(err)
	}
	if delivered.Load() != uint64(len(payload)) {
		t.Fatal("duplicate data billed as new delivery")
	}
}
