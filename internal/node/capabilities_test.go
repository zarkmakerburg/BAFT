package node

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	baftmetrics "github.com/zarkmakerburg/baft/internal/metrics"
	"github.com/zarkmakerburg/baft/internal/protocol"
	"github.com/zarkmakerburg/baft/internal/session"
)

func TestCapabilityRefusalIsCountedByTypedReasonAndAsHandshakeError(t *testing.T) {
	r := NewRuntime()
	r.observeCapabilities(protocol.NegotiatedCapabilities{}, nil) // success is not a refusal
	r.observeCapabilities(protocol.NegotiatedCapabilities{}, &protocol.CapabilityError{Reason: protocol.RefusalRequired, Missing: []string{protocol.CapNoiseV1}})
	r.observeCapabilities(protocol.NegotiatedCapabilities{}, errors.New("not typed"))
	got := r.capStats.snapshot()
	if got[protocol.RefusalRequired] != 1 || got[protocol.RefusalInvalid] != 1 || len(got) != 2 {
		t.Fatalf("%v", got)
	}
	if n := r.handshakeErrors.Load(); n != 2 {
		t.Fatalf("handshake errors %d", n)
	}
	var s baftmetrics.Snapshot
	r.fillCapabilityMetrics(&s, nil)
	rr := httptest.NewRecorder()
	baftmetrics.Handler(func() baftmetrics.Snapshot { return s }).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/metrics", nil))
	body := rr.Body.String()
	for _, want := range []string{
		`baft_capability_refusals_total{reason="required_missing"} 1`,
		`baft_capability_refusals_total{reason="invalid"} 1`,
		`baft_capability_refusals_total{reason="expected_missing"} 0`,
		`baft_capability_negotiated_sessions{capability="noise.v1"} 0`,
		`baft_capability_baseline_sessions 0`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in\n%s", want, body)
		}
	}
}

func TestMetricsCountNegotiatedAndBaselineSessionsWithoutPeerLabels(t *testing.T) {
	r := NewRuntime()
	var out bytes.Buffer
	p, err := session.New(session.Dialer, session.Carrier{In: bytes.NewReader(nil), Out: &out}, "urn:baft:node:ex-01", nil,
		session.Options{NodeID: "ir-01", ExpectedPeerNodeID: "ex-01", Capabilities: protocol.CapabilityOffer{Advertise: []string{protocol.CapNoiseV1}}})
	if err != nil {
		t.Fatal(err)
	}
	var s baftmetrics.Snapshot
	r.fillCapabilityMetrics(&s, []*session.Peer{p}) // handshake not done: not counted
	if len(s.CapabilitySessions) != 0 || s.CapabilityBaselineSessions != 0 {
		t.Fatalf("%+v", s)
	}
	if _, err := session.New(session.Dialer, session.Carrier{In: bytes.NewReader(nil), Out: &out}, "urn:baft:node:ex-01", nil,
		session.Options{NodeID: "ir-01", ExpectedPeerNodeID: "ex-01", Capabilities: protocol.CapabilityOffer{Advertise: []string{"nope"}}}); err == nil {
		t.Fatal("session accepted an unknown advertised capability")
	}
}
