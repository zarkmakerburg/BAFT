package bcc

import (
	"strings"
	"testing"
	"time"
)

func activeLegacyForCoexistence(t *testing.T, s *Store, now time.Time) Tunnel {
	t.Helper()
	legacy, err := s.CreateTunnel(TunnelRequest{EXNode: "ex-1", IRNode: "ir-1", Port: 8443, RouteListen: "127.0.0.1:1443"}, now)
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	legacy.Phase = TunnelActive
	s.st.Tunnels[legacy.ID] = legacy
	s.mu.Unlock()
	return legacy
}
func testCoexistenceRequest() TunnelRequest {
	r := scopedReq("arno-bg-1", "ex-2", "ir-1", 8443, "127.0.0.1:1444", "127.0.0.1:9202", "127.0.0.1:9302")
	r.AllowLegacyCoexistence = true
	return r
}
func TestExplicitLegacyCoexistenceKeepsOldTunnel(t *testing.T) {
	s := multiInstanceStore(t)
	now := time.Now()
	legacy := activeLegacyForCoexistence(t, s, now)
	// A deployable plan requires fresh authenticated agent heartbeats.
	s.mu.Lock()
	for _, id := range []string{"ir-1", "ex-2"} {
		n := s.st.Nodes[id]
		n.AgentSeen = now
		s.st.Nodes[id] = n
	}
	s.mu.Unlock()
	req := testCoexistenceRequest()
	unsafe := req
	unsafe.AllowLegacyCoexistence = false
	if _, err := s.BuildPlan(unsafe, now); err == nil || !strings.Contains(err.Error(), "legacy singleton") {
		t.Fatalf("implicit coexistence must fail closed: %v", err)
	}
	plan, err := s.BuildPlan(req, now)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.OK || plan.Replaces != "" || !plan.Request.AllowLegacyCoexistence {
		t.Fatalf("plan not isolated: %+v", plan)
	}
	warned := false
	for _, g := range plan.Gates {
		if g.Name == "keeps legacy tunnel active" && g.Status == "WARN" {
			warned = true
		}
	}
	if !warned {
		t.Fatal("operator must receive explicit coexistence warning")
	}
	if _, _, err = s.CreateTunnelFromPlan(unsafe, plan.Hash, now); err == nil {
		t.Fatal("plan consent flag can not be stripped without rejection")
	}
	scoped, _, err := s.CreateTunnelFromPlan(req, plan.Hash, now)
	if err != nil {
		t.Fatal(err)
	}
	if scoped.InstanceID != "arno-bg-1" {
		t.Fatalf("new tunnel instance %q", scoped.InstanceID)
	}
	s.mu.Lock()
	scoped.Phase = TunnelFinalizingEX
	s.st.Tunnels[scoped.ID] = scoped
	s.nextAfterFinalizeLocked(&scoped, now)
	s.st.Tunnels[scoped.ID] = scoped
	old := s.st.Tunnels[legacy.ID]
	newScoped := s.st.Tunnels[scoped.ID]
	s.mu.Unlock()
	if old.Phase != TunnelActive || newScoped.Phase != TunnelActive {
		t.Fatalf("legacy phase %s, scoped phase %s; must both stay active", old.Phase, newScoped.Phase)
	}
}
func TestExplicitLegacyCoexistenceRejectsPortConflicts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*TunnelRequest)
		want   string
	}{
		{"route collision", func(r *TunnelRequest) { r.RouteListen = "127.0.0.1:1443" }, "IR route listener"},
		{"metrics collision", func(r *TunnelRequest) { r.IRMetricsListen = "127.0.0.1:9191" }, "IR metrics listener"},
		{"missing instance", func(r *TunnelRequest) { r.InstanceID = "" }, "requires a distinct nonempty instance_id"},
		{"no metrics", func(r *TunnelRequest) { r.IRMetricsListen = "" }, "require ex_metrics_listen"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := multiInstanceStore(t)
			now := time.Now()
			activeLegacyForCoexistence(t, s, now)
			r := testCoexistenceRequest()
			tt.mutate(&r)
			_, err := s.BuildPlan(r, now)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q; got %v", tt.want, err)
			}
		})
	}
}
func TestLegacyCoexistenceDoesNotAllowUnscopedReplacementBypass(t *testing.T) {
	s := multiInstanceStore(t)
	now := time.Now()
	activeLegacyForCoexistence(t, s, now)
	r := TunnelRequest{EXNode: "ex-2", IRNode: "ir-1", AllowLegacyCoexistence: true}
	_, err := s.BuildPlan(r, now)
	if err == nil || !strings.Contains(err.Error(), "requires a distinct nonempty instance_id") {
		t.Fatalf("expected explicit instance guard, got %v", err)
	}
}
