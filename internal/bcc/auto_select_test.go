package bcc

import (
	"strings"
	"testing"
	"time"
)

func autoCandidate(now time.Time, carrier string, port int, rtt int64) AutoTransportCandidate {
	return AutoTransportCandidate{
		SourceNode: "ir-1", DestinationNode: "ex-1", Family: "4", Port: port,
		Class: PathFullData, State: PathMatrixPass, EvidenceAt: now.Add(-time.Minute),
		ConnectRTTMS: rtt, TricklePassed: true, CarrierID: carrier,
		InstalledAtSource: true, InstalledAtDestination: true,
		CapabilityEvidenceVerified: true, NegotiatedCapabilities: []string{"noise.v1", "carrier.h2"},
		ListenerAvailable: true,
	}
}

func TestSelectAutoTransportRanksOnlyVerifiedFullData(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	slow := autoCandidate(now, "h2", 8443, 40)
	fast := autoCandidate(now, "ws", 9443, 12)
	got := SelectAutoTransport(AutoTransportRequest{
		SourceNode: "ir-1", DestinationNode: "ex-1", Family: "4", Now: now,
		RequiredCapabilities: []string{"noise.v1"},
	}, []AutoTransportCandidate{slow, fast})
	if got.Selected == nil {
		t.Fatalf("expected candidate, rejected: %+v", got.Rejected)
	}
	if got.Selected.CarrierID != "ws" || got.Selected.Port != 9443 {
		t.Fatalf("selected %+v, want ws:9443", *got.Selected)
	}
}

func TestSelectAutoTransportRefusesDegradedOrStaleEvidence(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		mutate func(*AutoTransportCandidate)
	}{
		{"connect-only", func(c *AutoTransportCandidate) { c.Class = PathConnectOnly }},
		{"handshake-only", func(c *AutoTransportCandidate) { c.Class = PathHandshakeOnly }},
		{"byte-ceiling", func(c *AutoTransportCandidate) { c.Class = PathByteCeiling }},
		{"failed-trickle", func(c *AutoTransportCandidate) { c.TricklePassed = false }},
		{"stale", func(c *AutoTransportCandidate) { c.EvidenceAt = now.Add(-pathDiscoveryFreshFor - time.Second) }},
		{"future-dated", func(c *AutoTransportCandidate) { c.EvidenceAt = now.Add(time.Second) }},
		{"wrong-pair", func(c *AutoTransportCandidate) { c.DestinationNode = "ex-2" }},
		{"wrong-family", func(c *AutoTransportCandidate) { c.Family = "6" }},
		{"unverified-capabilities", func(c *AutoTransportCandidate) { c.CapabilityEvidenceVerified = false }},
		{"missing-required-capability", func(c *AutoTransportCandidate) { c.NegotiatedCapabilities = []string{"carrier.h2"} }},
		{"not-installed-on-source", func(c *AutoTransportCandidate) { c.InstalledAtSource = false }},
		{"not-installed-on-destination", func(c *AutoTransportCandidate) { c.InstalledAtDestination = false }},
		{"listener-conflict", func(c *AutoTransportCandidate) { c.ListenerAvailable = false }},
		{"unverified-multiplex", func(c *AutoTransportCandidate) { c.ListenerAvailable = false; c.SharedListener = true }},
		{"uTLS-required-but-unsupported", func(c *AutoTransportCandidate) { c.UTLSSupported = false }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := autoCandidate(now, "h2", 8443, 10)
			tc.mutate(&c)
			got := SelectAutoTransport(AutoTransportRequest{
				SourceNode: "ir-1", DestinationNode: "ex-1", Family: "4",
				Now: now, RequiredCapabilities: []string{"noise.v1"},
				RequireUTLS: tc.name == "uTLS-required-but-unsupported",
			}, []AutoTransportCandidate{c})
			if got.Selected != nil {
				t.Fatalf("unsafe candidate selected: %+v", *got.Selected)
			}
			if len(got.Rejected) != 1 {
				t.Fatalf("rejections=%d, want 1: %+v", len(got.Rejected), got.Rejected)
			}
		})
	}
}

func TestSelectAutoTransportExplicitCarrierNeverDowngrades(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	c := autoCandidate(now, "h2", 8443, 10)
	got := SelectAutoTransport(AutoTransportRequest{
		SourceNode: "ir-1", DestinationNode: "ex-1", Family: "4",
		RequestedCarrier: "ws", Now: now,
	}, []AutoTransportCandidate{c})
	if got.Selected != nil {
		t.Fatalf("silently changed explicit carrier: %+v", *got.Selected)
	}
	if len(got.Rejected) != 1 || !strings.Contains(got.Rejected[0].Reason, "explicitly selected carrier") {
		t.Fatalf("unexpected refusal: %+v", got.Rejected)
	}
}

func TestSelectAutoTransportRequiresVerifiedMultiplexForSharedListener(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	c := autoCandidate(now, "h2", 443, 10)
	c.ListenerAvailable = false
	c.SharedListener = true
	c.MultiplexVerified = true
	got := SelectAutoTransport(AutoTransportRequest{
		SourceNode: "ir-1", DestinationNode: "ex-1", Family: "4", Now: now,
	}, []AutoTransportCandidate{c})
	if got.Selected == nil {
		t.Fatalf("verified shared listener should be eligible: %+v", got.Rejected)
	}
}

func TestSelectAutoTransportDeterministicTieBreak(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	z := autoCandidate(now, "z-carrier", 9443, 20)
	a := autoCandidate(now, "a-carrier", 8443, 20)
	for i := 0; i < 5; i++ {
		got := SelectAutoTransport(AutoTransportRequest{
			SourceNode: "ir-1", DestinationNode: "ex-1", Family: "4", Now: now,
		}, []AutoTransportCandidate{z, a})
		if got.Selected == nil || got.Selected.CarrierID != "a-carrier" {
			t.Fatalf("iteration %d selected %+v, want deterministic a-carrier", i, got.Selected)
		}
	}
}

func TestResolveAutoPublicClientPortDefaultsToTargetAndFlagsOverride(t *testing.T) {
	port, mismatch, err := ResolveAutoPublicClientPort(54619, 0)
	if err != nil || port != 54619 || mismatch {
		t.Fatalf("default: port=%d mismatch=%v err=%v", port, mismatch, err)
	}
	port, mismatch, err = ResolveAutoPublicClientPort(54619, 443)
	if err != nil || port != 443 || !mismatch {
		t.Fatalf("override: port=%d mismatch=%v err=%v", port, mismatch, err)
	}
	if _, _, err := ResolveAutoPublicClientPort(0, 443); err == nil {
		t.Fatal("invalid target port accepted")
	}
	if _, _, err := ResolveAutoPublicClientPort(54619, 65536); err == nil {
		t.Fatal("invalid explicit port accepted")
	}
}
