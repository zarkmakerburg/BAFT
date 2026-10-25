package session

import (
	"context"
	"errors"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/protocol"
	"github.com/zarkmakerburg/baft/internal/protocol/legacyv037"
	"github.com/zarkmakerburg/baft/internal/routes"
)

type capObs struct {
	mu   sync.Mutex
	n    []protocol.NegotiatedCapabilities
	errs []error
}

func (o *capObs) observe(n protocol.NegotiatedCapabilities, err error) {
	o.mu.Lock()
	o.n, o.errs = append(o.n, n), append(o.errs, err)
	o.mu.Unlock()
}
func (o *capObs) last(t *testing.T) (protocol.NegotiatedCapabilities, error) {
	t.Helper()
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.n) != 1 {
		t.Fatalf("observer called %d times, want exactly 1", len(o.n))
	}
	return o.n[0], o.errs[0]
}

// capTCPPair returns a connected loopback TCP pair: unlike net.Pipe it is
// buffered, like a real carrier, so simultaneous writers cannot deadlock.
func capTCPPair(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	type res struct {
		c   net.Conn
		err error
	}
	ch := make(chan res, 1)
	go func() { c, err := ln.Accept(); ch <- res{c, err} }()
	a, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	r := <-ch
	if r.err != nil {
		t.Fatal(r.err)
	}
	return a, r.c
}

func capTable(t *testing.T) *routes.Table {
	t.Helper()
	tbl, err := routes.New([]routes.Route{{ID: "main", Target: "127.0.0.1:2443", AllowedPeers: map[string]struct{}{"urn:baft:node:ir-01": {}}}})
	if err != nil {
		t.Fatal(err)
	}
	return tbl
}

func capPeer(t *testing.T, role Role, c net.Conn, off protocol.CapabilityOffer, obs *capObs) *Peer {
	t.Helper()
	o := Options{NodeID: "ir-01", ExpectedPeerNodeID: "ex-01", ProfileID: "secure-fast", ProfileVersion: 1, ConfigRevision: "config-v1", Capabilities: off}
	var tbl *routes.Table
	peerID := "urn:baft:node:ex-01"
	if role == Listener {
		o.NodeID, o.ExpectedPeerNodeID = "ex-01", "ir-01"
		peerID = "urn:baft:node:ir-01"
		tbl = capTable(t)
	}
	if obs != nil {
		o.CapabilityObserver = obs.observe
	}
	p, err := New(role, Carrier{In: c, Out: c}, peerID, tbl, o)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func runPeer(ctx context.Context, p *Peer) <-chan error {
	ch := make(chan error, 1)
	go func() { ch <- p.Run(ctx) }()
	return ch
}

func waitErr(t *testing.T, ch <-chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("peer did not exit")
		return nil
	}
}

func ready(t *testing.T, p *Peer) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.waitReady(ctx); err != nil {
		t.Fatalf("handshake did not complete: %v", err)
	}
}

func asCapErr(t *testing.T, err error, reason string) *protocol.CapabilityError {
	t.Helper()
	var ce *protocol.CapabilityError
	if !errors.As(err, &ce) || ce.Reason != reason {
		t.Fatalf("want CapabilityError(%s), got %v", reason, err)
	}
	return ce
}

var (
	offerNoiseH2 = protocol.CapabilityOffer{
		Advertise: []string{protocol.CapCarrierH2, protocol.CapNoiseV1},
		Expect:    []string{protocol.CapNoiseV1},
	}
)

func TestNewToNewNegotiatesIntersectionAndSurfacesIt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, b := capTCPPair(t)
	defer a.Close()
	defer b.Close()
	dObs, lObs := &capObs{}, &capObs{}
	dOffer := protocol.CapabilityOffer{
		Advertise: []string{protocol.CapCarrierH2, protocol.CapNoiseV1, protocol.CapRecordShapeV1},
		Expect:    []string{protocol.CapNoiseV1},
	}
	d := capPeer(t, Dialer, a, dOffer, dObs)
	l := capPeer(t, Listener, b, offerNoiseH2, lObs)
	lc, dc := runPeer(ctx, l), runPeer(ctx, d)
	ready(t, d)
	ready(t, l)
	want := []string{protocol.CapCarrierH2, protocol.CapNoiseV1} // shape.v1 is optional and the listener lacks it
	for name, p := range map[string]*Peer{"dialer": d, "listener": l} {
		n, ok := p.NegotiatedCapabilities()
		if !ok || !reflect.DeepEqual(n.Enabled, want) || n.Baseline {
			t.Errorf("%s: %+v ok=%v", name, n, ok)
		}
	}
	for name, o := range map[string]*capObs{"dialer": dObs, "listener": lObs} {
		if n, err := o.last(t); err != nil || !reflect.DeepEqual(n.Enabled, want) {
			t.Errorf("%s observer: %+v %v", name, n, err)
		}
	}
	cancel()
	a.Close()
	b.Close()
	waitErr(t, lc)
	waitErr(t, dc)
}

func TestNoOptionsIsExactlyTheV037Baseline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, b := capTCPPair(t)
	defer a.Close()
	defer b.Close()
	d := capPeer(t, Dialer, a, protocol.CapabilityOffer{}, nil)
	l := capPeer(t, Listener, b, protocol.CapabilityOffer{}, nil)
	runPeer(ctx, l)
	runPeer(ctx, d)
	ready(t, d)
	ready(t, l)
	n, ok := d.NegotiatedCapabilities()
	if !ok || !n.Baseline || len(n.Enabled) != 0 {
		t.Fatalf("%+v %v", n, ok)
	}
}

func TestListenerRefusesDialerLackingRequiredCapabilityExplicitly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, b := capTCPPair(t)
	defer a.Close()
	defer b.Close()
	lObs := &capObs{}
	lOffer := protocol.CapabilityOffer{
		Advertise: []string{protocol.CapCarrierH2, protocol.CapNoiseV1},
		Require:   []string{protocol.CapNoiseV1},
	}
	// dialer advertises h2 only (no Noise).
	d := capPeer(t, Dialer, a, protocol.CapabilityOffer{Advertise: []string{protocol.CapCarrierH2}}, nil)
	l := capPeer(t, Listener, b, lOffer, lObs)
	lc := runPeer(ctx, l)
	runPeer(ctx, d)
	ce := asCapErr(t, waitErr(t, lc), protocol.RefusalRequired)
	if !reflect.DeepEqual(ce.Missing, []string{protocol.CapNoiseV1}) {
		t.Fatalf("%+v", ce)
	}
	if _, err := lObs.last(t); err == nil {
		t.Fatal("observer not told about the refusal")
	}
	if l.isReady() || d.isReady() {
		t.Fatal("session became ready despite refusal")
	}
	if _, ok := l.NegotiatedCapabilities(); ok {
		t.Fatal("negotiation reported complete after refusal")
	}
}

func TestDialerRefusesListenerLackingExpectedCapability(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, b := capTCPPair(t)
	defer a.Close()
	defer b.Close()
	// Dialer is configured for h2 primary + ws fallback; the listener only serves h2.
	dOffer := protocol.CapabilityOffer{
		Advertise: []string{protocol.CapCarrierH2, protocol.CapCarrierWS, protocol.CapNoiseV1},
		Expect:    []string{protocol.CapCarrierH2, protocol.CapCarrierWS, protocol.CapNoiseV1},
	}
	d := capPeer(t, Dialer, a, dOffer, nil)
	l := capPeer(t, Listener, b, offerNoiseH2, nil)
	dc := runPeer(ctx, d)
	runPeer(ctx, l)
	ce := asCapErr(t, waitErr(t, dc), protocol.RefusalExpected)
	if !reflect.DeepEqual(ce.Missing, []string{protocol.CapCarrierWS}) {
		t.Fatalf("%+v", ce)
	}
}

func TestDialerRefusesUnsolicitedAckCapabilities(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, b := capTCPPair(t)
	defer a.Close()
	defer b.Close()
	d := capPeer(t, Dialer, a, protocol.CapabilityOffer{}, nil)
	dc := runPeer(ctx, d)
	hello, err := protocol.Decode(b)
	if err != nil || hello.Type != protocol.TypeHello {
		t.Fatalf("%v %v", err, hello.Type)
	}
	h, err := protocol.DecodeHello(hello.Payload)
	if err != nil {
		t.Fatal(err)
	}
	ack := protocol.HelloAck{SelectedProtocol: 1, SessionID: h.SessionID, Epoch: h.Epoch, PeerBootID: h.BootID,
		AcceptedProfile:  protocol.AcceptedProfile{ID: "secure-fast", Version: 1},
		NegotiatedLimits: protocol.NegotiatedLimits{MaxFramePayloadBytes: protocol.MaxPayloadSize},
		Capabilities:     []string{protocol.CapNoiseV1}}
	payload, _ := protocol.EncodeControl(ack)
	go protocol.Encode(b, protocol.Frame{Type: protocol.TypeHelloAck, Payload: payload})
	asCapErr(t, waitErr(t, dc), protocol.RefusalUnsolicited)
}

// ---- v0.3.7 interoperability: the OLD handshake code against the new Peer ---

// legacyListener behaves like a v0.3.7 listener's handshake: it validates HELLO
// with the frozen v0.3.7 decoder, answers with a v0.3.7-shaped HELLO_ACK that
// the v0.3.7 decoder itself accepts, and sends READY.
func legacyListener(t *testing.T, c net.Conn, gotHello chan<- legacyv037.Hello) error {
	fr, err := protocol.Decode(c)
	if err != nil {
		return err
	}
	h, err := legacyv037.DecodeHello(fr.Payload)
	if err != nil {
		return err
	}
	gotHello <- h
	ackB, err := legacyv037.EncodeControl(legacyv037.HelloAck{SelectedProtocol: 1, SessionID: h.SessionID, Epoch: h.Epoch, PeerBootID: "00112233445566778899aabbccddeeff",
		AcceptedProfile:  legacyv037.AcceptedProfile{ID: h.ProfileID, Version: h.ProfileVersion},
		NegotiatedLimits: legacyv037.NegotiatedLimits{MaxFramePayloadBytes: protocol.MaxPayloadSize, MaxFlowsPerShard: 64, ReceiveInitialBytes: 1 << 20, ReceiveMaxBytes: 16 << 20, RetentionMS: 30000}})
	if err != nil {
		return err
	}
	if _, err := legacyv037.DecodeHelloAck(ackB); err != nil {
		return err
	}
	if err := protocol.Encode(c, protocol.Frame{Type: protocol.TypeHelloAck, Payload: ackB}); err != nil {
		return err
	}
	if err := protocol.Encode(c, protocol.Frame{Type: protocol.TypeReady, Payload: []byte(`{"snapshot_id":null}`)}); err != nil {
		return err
	}
	for { // drain until the peer closes
		if _, err := protocol.Decode(c); err != nil {
			return nil
		}
	}
}

// legacyDialer behaves like a v0.3.7 dialer: HELLO with `capabilities: []`,
// strict v0.3.7 HELLO_ACK validation, then READY.
func legacyDialer(t *testing.T, c net.Conn, ackCaps chan<- string) error {
	helloB, err := legacyv037.EncodeControl(legacyv037.Hello{ProtocolMin: 1, ProtocolMax: 1, NodeID: "ir-01",
		BootID: "00112233445566778899aabbccddeeff", SessionID: "ffeeddccbbaa99887766554433221100", Epoch: "1", Mode: "new",
		ProfileID: "secure-fast", ProfileVersion: 1, ConfigRevision: "config-v1", Capabilities: []string{}})
	if err != nil {
		return err
	}
	if err := protocol.Encode(c, protocol.Frame{Type: protocol.TypeHello, Payload: helloB}); err != nil {
		return err
	}
	fr, err := protocol.Decode(c)
	if err != nil {
		return err
	}
	ackCaps <- string(fr.Payload)
	if _, err := legacyv037.DecodeHelloAck(fr.Payload); err != nil { // DisallowUnknownFields
		return err
	}
	if err := protocol.Encode(c, protocol.Frame{Type: protocol.TypeReady, Payload: []byte(`{"snapshot_id":null}`)}); err != nil {
		return err
	}
	for {
		if _, err := protocol.Decode(c); err != nil {
			return nil
		}
	}
}

func TestNewDialerInteroperatesWithV037Listener(t *testing.T) {
	for name, off := range map[string]protocol.CapabilityOffer{
		"advertise only":      {Advertise: []string{protocol.CapCarrierH2, protocol.CapNoiseV1}},
		"expect is waived":    offerNoiseH2,
		"nothing configured":  {},
		"all known advertise": {Advertise: protocol.KnownCapabilities()},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			a, b := capTCPPair(t)
			defer a.Close()
			defer b.Close()
			hello := make(chan legacyv037.Hello, 1)
			legacyDone := make(chan error, 1)
			go func() { legacyDone <- legacyListener(t, b, hello) }()
			d := capPeer(t, Dialer, a, off, nil)
			runPeer(ctx, d)
			ready(t, d)
			h := <-hello
			if len(h.Capabilities) != len(off.Advertise) {
				t.Fatalf("v0.3.7 listener saw %v", h.Capabilities)
			}
			n, ok := d.NegotiatedCapabilities()
			if !ok || !n.Baseline || len(n.Enabled) != 0 {
				t.Fatalf("must negotiate the baseline against v0.3.7: %+v", n)
			}
			cancel()
			a.Close()
			if err := <-legacyDone; err != nil {
				t.Fatalf("v0.3.7 listener code rejected the new dialer: %v", err)
			}
		})
	}
}

func TestNewDialerWithRequirementRefusesV037ListenerExplicitly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, b := capTCPPair(t)
	defer a.Close()
	defer b.Close()
	hello := make(chan legacyv037.Hello, 1)
	go legacyListener(t, b, hello)
	obs := &capObs{}
	off := offerNoiseH2
	off.Require = []string{protocol.CapNoiseV1}
	d := capPeer(t, Dialer, a, off, obs)
	dc := runPeer(ctx, d)
	asCapErr(t, waitErr(t, dc), protocol.RefusalRequired)
	if d.isReady() {
		t.Fatal("dialer became ready against a peer lacking a required capability")
	}
	if _, err := obs.last(t); err == nil {
		t.Fatal("refusal not observed")
	}
}

func TestV037DialerInteroperatesWithNewListener(t *testing.T) {
	for name, off := range map[string]protocol.CapabilityOffer{
		"nothing configured": {},
		"expect is waived":   offerNoiseH2,
		"all known":          {Advertise: protocol.KnownCapabilities(), Expect: []string{protocol.CapNoiseV1}},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			a, b := capTCPPair(t)
			defer a.Close()
			defer b.Close()
			ackCaps := make(chan string, 1)
			legacyDone := make(chan error, 1)
			go func() { legacyDone <- legacyDialer(t, a, ackCaps) }()
			l := capPeer(t, Listener, b, off, nil)
			runPeer(ctx, l)
			ready(t, l)
			// The ack the v0.3.7 decoder validated must carry no capabilities field.
			raw := <-ackCaps
			if got, err := protocol.DecodeHelloAck([]byte(raw)); err != nil || got.Capabilities != nil {
				t.Fatalf("ack to a v0.3.7 dialer leaked capabilities: %s (%v)", raw, err)
			}
			n, ok := l.NegotiatedCapabilities()
			if !ok || !n.Baseline || len(n.Enabled) != 0 {
				t.Fatalf("%+v", n)
			}
			cancel()
			a.Close()
			if err := <-legacyDone; err != nil {
				t.Fatalf("v0.3.7 dialer code rejected the new listener: %v", err)
			}
		})
	}
}

func TestNewListenerWithRequirementRefusesV037DialerWithoutAck(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, b := capTCPPair(t)
	defer a.Close()
	defer b.Close()
	ackCaps := make(chan string, 1)
	legacyDone := make(chan error, 1)
	go func() { legacyDone <- legacyDialer(t, a, ackCaps) }()
	off := offerNoiseH2
	off.Require = []string{protocol.CapNoiseV1}
	l := capPeer(t, Listener, b, off, nil)
	lc := runPeer(ctx, l)
	asCapErr(t, waitErr(t, lc), protocol.RefusalRequired)
	b.Close() // the node closes the carrier on a refused session
	select {
	case raw := <-ackCaps:
		t.Fatalf("a refusing listener must not send HELLO_ACK, sent %s", raw)
	default:
	}
	if err := <-legacyDone; err == nil {
		t.Fatal("legacy dialer unexpectedly completed")
	}
}
