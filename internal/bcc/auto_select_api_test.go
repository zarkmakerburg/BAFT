package bcc

import (
	"strings"
	"testing"
	"time"
)

// autoSelectStore: IR "ir" dials EX "ex"; both agents polled just now with the
// given capability tokens (header spelling).
func autoSelectStore(t *testing.T, now time.Time, irCaps, exCaps []string) *Store {
	t.Helper()
	s := newGenericDiscoveryStore(t, "ir", "ex")
	s.mu.Lock()
	for id, caps := range map[string][]string{"ir": irCaps, "ex": exCaps} {
		n := s.st.Nodes[id]
		n.AgentSeen, n.Capabilities = now, caps
		s.st.Nodes[id] = n
	}
	s.mu.Unlock()
	return s
}

func putTrickleProbe(s *Store, id string, port int, class string, rtt int64, at time.Time) {
	putGraphProbe(s, id, "ir", "ex", "4", "192.0.2.2", port, class, rtt, at)
	p := s.st.PathProbes[id]
	p.Candidates[0].TricklePassed = true
	s.st.PathProbes[id] = p
}

var fullCaps = []string{"carrier-h2", "carrier-ws", "noise-v1"}

func TestAutoSelectPicksFreshFullDataOnAnInstalledCarrier(t *testing.T) {
	now := time.Now().UTC()
	s := autoSelectStore(t, now, fullCaps, fullCaps)
	s.mu.Lock()
	putTrickleProbe(s, "p443", 443, PathFullData, 30, now)
	putTrickleProbe(s, "p8443", 8443, PathFullData, 12, now)
	s.mu.Unlock()
	sel, err := s.AutoSelectTransport(AutoSelectRequest{EXNode: "ex", IRNode: "ir", Family: "4"}, now)
	if err != nil || sel.Selected == nil {
		t.Fatalf("err=%v sel=%+v", err, sel)
	}
	if sel.Selected.Port != 8443 || sel.Selected.CarrierID != "h2" {
		t.Fatalf("selected %+v, want h2:8443 (lowest RTT, deterministic carrier order)", *sel.Selected)
	}
}

func TestAutoSelectRefusesWithoutEvidence(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name          string
		irCaps        []string
		class         string
		seenAgo       time.Duration
		occupyPort    bool
		probe         bool
		reasonContain string
	}{
		{name: "binary without carrier capabilities", irCaps: []string{"operational-ingress-v1"}, class: PathFullData, probe: true},
		{name: "stale agent report", irCaps: fullCaps, class: PathFullData, seenAgo: ingressCapabilityMaxAge + time.Second, probe: true},
		{name: "byte ceiling only", irCaps: fullCaps, class: PathByteCeiling, probe: true},
		{name: "connect only", irCaps: fullCaps, class: PathConnectOnly, probe: true},
		{name: "port held by a tunnel", irCaps: fullCaps, class: PathFullData, occupyPort: true, probe: true},
		{name: "no probe", irCaps: fullCaps, reasonContain: "run path discovery"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := autoSelectStore(t, now, tc.irCaps, fullCaps)
			s.mu.Lock()
			if tc.seenAgo > 0 {
				n := s.st.Nodes["ir"]
				n.AgentSeen = now.Add(-tc.seenAgo)
				s.st.Nodes["ir"] = n
			}
			if tc.probe {
				putTrickleProbe(s, "p", 443, tc.class, 10, now)
			}
			if tc.occupyPort {
				if s.st.Tunnels == nil {
					s.st.Tunnels = map[string]Tunnel{}
				}
				s.st.Tunnels["t1"] = Tunnel{ID: "t1", EXNode: "ex", IRNode: "other", Port: 443, Phase: TunnelActive}
			}
			s.mu.Unlock()
			sel, err := s.AutoSelectTransport(AutoSelectRequest{EXNode: "ex", IRNode: "ir", Family: "4"}, now)
			if err != nil {
				t.Fatal(err)
			}
			if sel.Selected != nil {
				t.Fatalf("selected %+v without sufficient evidence", *sel.Selected)
			}
			if sel.Reason == "" || (tc.reasonContain != "" && !strings.Contains(sel.Reason, tc.reasonContain)) {
				t.Fatalf("reason %q", sel.Reason)
			}
		})
	}
}

func TestAutoSelectWebSocketNeedsNoiseAndNeverDowngrades(t *testing.T) {
	now := time.Now().UTC()
	noNoise := []string{"carrier-h2", "carrier-ws"}
	s := autoSelectStore(t, now, noNoise, noNoise)
	s.mu.Lock()
	putTrickleProbe(s, "p", 443, PathFullData, 10, now)
	s.mu.Unlock()
	sel, _ := s.AutoSelectTransport(AutoSelectRequest{EXNode: "ex", IRNode: "ir", Family: "4", Carrier: "ws"}, now)
	if sel.Selected != nil {
		t.Fatalf("ws selected without noise.v1: %+v", *sel.Selected)
	}
	s2 := autoSelectStore(t, now, fullCaps, fullCaps)
	s2.mu.Lock()
	putTrickleProbe(s2, "p", 443, PathFullData, 10, now)
	s2.mu.Unlock()
	sel, _ = s2.AutoSelectTransport(AutoSelectRequest{EXNode: "ex", IRNode: "ir", Family: "4", Carrier: "ws"}, now)
	if sel.Selected == nil || sel.Selected.CarrierID != "ws" {
		t.Fatalf("explicit ws with noise on both nodes: %+v", sel)
	}
	if _, err := s2.AutoSelectTransport(AutoSelectRequest{EXNode: "ex", IRNode: "ir", Family: "4", Carrier: "quic"}, now); err == nil {
		t.Fatal("unknown carrier accepted")
	}
}

func TestAutoSelectEvidenceIsBoundAndRevalidated(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	s := autoSelectStore(t, now, fullCaps, fullCaps)
	s.mu.Lock()
	putTrickleProbe(s, "p443", 443, PathFullData, 30, now)
	putTrickleProbe(s, "p8443", 8443, PathFullData, 12, now)
	s.mu.Unlock()

	selection, err := s.AutoSelectTransport(AutoSelectRequest{EXNode: "ex", IRNode: "ir", Family: "4", Carrier: "h2"}, now)
	if err != nil || selection.Selected == nil {
		t.Fatalf("auto-select failed: selection=%+v err=%v", selection, err)
	}
	evidence := *selection.Selected
	req := TunnelRequest{EXNode: "ex", IRNode: "ir", Port: evidence.Port, AutoSelectEvidence: &evidence}
	if err := s.validateAutoSelectEvidence(req, now); err != nil {
		t.Fatalf("fresh selected evidence rejected: %v", err)
	}

	// Evidence cannot be attached to another requested port or node pair.
	mismatch := req
	mismatch.Port++
	if err := s.validateAutoSelectEvidence(mismatch, now); err == nil {
		t.Fatal("evidence for one port authorized a different requested port")
	}
	mismatch = req
	mismatch.EXNode = "another-ex"
	if err := s.validateAutoSelectEvidence(mismatch, now); err == nil {
		t.Fatal("evidence for one ordered node pair authorized another pair")
	}

	// A re-probe changes the evidence timestamp and RTT; the previous review
	// binding must fail closed even if the port remains selectable.
	s.mu.Lock()
	putTrickleProbe(s, "p8443", 8443, PathFullData, 11, now.Add(time.Second))
	s.mu.Unlock()
	if err := s.validateAutoSelectEvidence(req, now.Add(time.Second)); err == nil {
		t.Fatal("stale selection binding survived a changed path observation")
	}

	// Auto-select evidence is part of the immutable plan request/hash.
	p1 := Plan{Request: req}
	p2Req := req
	changedEvidence := evidence
	changedEvidence.ConnectRTTMS++
	p2Req.AutoSelectEvidence = &changedEvidence
	if planHash(Plan{Request: p2Req}) == planHash(p1) {
		t.Fatal("plan hash ignored changed auto-select evidence")
	}
}

func TestAutoSelectAPIIsAdminOnlyAndReadOnly(t *testing.T) {
	f := newTunnelHTTPFixture(t)
	req := AutoSelectRequest{EXNode: "ex", IRNode: "ir", Family: "4"}
	f.call("POST", "/api/tunnels/auto-select", "wrong-token", req, 401, nil)
	f.call("GET", "/api/tunnels/auto-select", tunnelHTTPAdmin, nil, 405, nil)
	f.call("POST", "/api/tunnels/auto-select", tunnelHTTPAdmin, AutoSelectRequest{EXNode: "ex", IRNode: "ir", Family: "4", Carrier: "tcp"}, 400, nil)
	before := len(f.store.ListJobs())
	var sel AutoTransportSelection
	f.call("POST", "/api/tunnels/auto-select", tunnelHTTPAdmin, req, 200, &sel)
	if sel.Selected != nil || sel.Reason == "" {
		t.Fatalf("selection without evidence: %+v", sel)
	}
	if after := len(f.store.ListJobs()); after != before {
		t.Fatalf("auto-select created %d jobs", after-before)
	}
}
