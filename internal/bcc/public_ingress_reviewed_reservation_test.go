package bcc

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A desired reservation must be bound to the precise reviewed tunnel plan,
// not merely to a mutable tunnel ID or the operator's unreviewed UI port.
func TestReserveReviewedPublicIngressBindsExactPlanAndPortDurably(t *testing.T) {
	s, spec := reservationStore(t)
	s.mu.Lock()
	tunnel := s.st.Tunnels[spec.TunnelID]
	tunnel.PlanHash = "reviewed-hash-1"
	tunnel.ClientPort = spec.PublicPort
	s.st.Tunnels[spec.TunnelID] = tunnel
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()
	now := time.Now()
	for _, tc := range []struct {
		name, hash string
		port       int
	}{
		{"missing plan", "", spec.PublicPort},
		{"outdated plan", "reviewed-hash-old", spec.PublicPort},
		{"wrong public port", "reviewed-hash-1", 54620},
	} {
		probe := spec
		probe.PublicPort = tc.port
		if _, err := s.ReserveReviewedPublicIngress(probe, tc.hash, now, ingressAudit()); err == nil {
			t.Fatalf("%s accepted", tc.name)
		}
		if got, _ := s.ListPublicIngressReservations(); len(got) != 0 {
			t.Fatalf("%s persisted reservation: %+v", tc.name, got)
		}
	}
	got, err := s.ReserveReviewedPublicIngress(spec, "reviewed-hash-1", now, ingressAudit())
	if err != nil || got.Spec != spec {
		t.Fatalf("reviewed reservation: %+v %v", got, err)
	}
	reopened, err := OpenStore(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	saved, err := reopened.ListPublicIngressReservations()
	if err != nil || len(saved) != 1 || saved[0] != got {
		t.Fatalf("durability: %+v %v", saved, err)
	}
}

func TestReserveReviewedPublicIngressRejectsNonActiveAndUnreviewedTunnel(t *testing.T) {
	s, spec := reservationStore(t)
	s.mu.Lock()
	tunnel := s.st.Tunnels[spec.TunnelID]
	tunnel.PlanHash = "hash"
	tunnel.ClientPort = spec.PublicPort
	tunnel.Phase = TunnelRollingBack
	s.st.Tunnels[spec.TunnelID] = tunnel
	s.mu.Unlock()
	if _, err := s.ReserveReviewedPublicIngress(spec, "hash", time.Now(), ingressAudit()); err == nil {
		t.Fatal("rollback tunnel accepted")
	}
}

func TestReviewedTunnelPlanBindsCanonicalClientPort(t *testing.T) {
	s := multiInstanceStore(t)
	now := time.Now().UTC()
	s.mu.Lock()
	for _, id := range []string{"ex-1", "ir-1"} {
		n := s.st.Nodes[id]
		n.AgentSeen = now
		s.st.Nodes[id] = n
	}
	s.mu.Unlock()
	req := TunnelRequest{EXNode: "ex-1", IRNode: "ir-1", ClientPort: 443}
	p, err := s.BuildPlan(req, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.Request.ClientPort != 443 {
		t.Fatalf("plan dropped client port: %+v", p.Request)
	}
	changed := req
	changed.ClientPort = 2053
	p2, err := s.BuildPlan(changed, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.Hash == p2.Hash {
		t.Fatal("different client config ports have same reviewed plan hash")
	}
	created, _, err := s.CreateTunnelFromPlan(req, p.Hash, now)
	if err != nil || created.ClientPort != 443 || created.PlanHash != p.Hash {
		t.Fatalf("reviewed tunnel did not durably preserve canonical port and plan: %+v %v", created, err)
	}
	reopened, err := OpenStore(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	persisted, ok := reopened.GetTunnel(created.ID)
	if !ok || persisted.ClientPort != 443 || persisted.PlanHash != p.Hash {
		t.Fatalf("restart lost reviewed client port: %+v", persisted)
	}
	if _, err := s.BuildPlan(TunnelRequest{EXNode: "ex-1", IRNode: "ir-1", ClientPort: 22}, now); err == nil {
		t.Fatal("SSH client port admitted")
	}
	if _, err := s.BuildPlan(TunnelRequest{EXNode: "ex-1", IRNode: "ir-1", ClientPort: 1443}, now); err == nil {
		t.Fatal("ingress port aliases IR internal loopback route")
	}
}

func TestReviewedClientPortClaimBlocksSiblingBeforeIngressApply(t *testing.T) {
	s := multiInstanceStore(t)
	now := time.Now()
	first := scopedReq("tenant-one", "ex-1", "ir-1", 8443, "127.0.0.1:1443", "127.0.0.1:9202", "127.0.0.1:9302")
	first.ClientPort = 54619
	second := scopedReq("tenant-two", "ex-2", "ir-1", 8443, "127.0.0.1:1444", "127.0.0.1:9202", "127.0.0.1:9303")
	second.ClientPort = 54619
	if _, err := s.CreateTunnel(first, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTunnel(second, now); err == nil {
		t.Fatal("second tunnel silently claimed same IR client port")
	}
}

func TestReviewedIngressReservationAPIUsesPersistedPlanPortAndRemainsDesiredOnly(t *testing.T) {
	s, spec := reservationStore(t)
	s.mu.Lock()
	tunnel := s.st.Tunnels[spec.TunnelID]
	tunnel.PlanHash = "approved-plan"
	tunnel.ClientPort = spec.PublicPort
	s.st.Tunnels[tunnel.ID] = tunnel
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()
	app, err := NewServer(s, "admin")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := "/api/ingress/reviewed-reservations"
	makeReq := func(token, hash string, port int) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, authReq(http.MethodPost, endpoint, token, map[string]any{"tunnel_id": spec.TunnelID, "plan_hash": hash, "client_port": port}))
		return rr
	}
	if rr := makeReq("", "approved-plan", spec.PublicPort); rr.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous mutation: %d %s", rr.Code, rr.Body.String())
	}
	if rr := makeReq("admin", "wrong-plan", spec.PublicPort); rr.Code != http.StatusBadRequest {
		t.Fatalf("stale plan admitted: %d %s", rr.Code, rr.Body.String())
	}
	if rr := makeReq("admin", "approved-plan", spec.PublicPort+1); rr.Code != http.StatusBadRequest {
		t.Fatalf("changed client port admitted: %d %s", rr.Code, rr.Body.String())
	}
	rr := makeReq("admin", "approved-plan", spec.PublicPort)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "\"desired_only\":true") || !strings.Contains(rr.Body.String(), "\"ready_to_apply\":false") {
		t.Fatalf("reviewed reservation: %d %s", rr.Code, rr.Body.String())
	}
	if got, _ := s.ListPublicIngressReservations(); len(got) != 1 || got[0].Spec != spec {
		t.Fatalf("API failed to persist exact desired tuple: %+v", got)
	}
}

func TestReviewedClientPortClaimAlsoProtectsAgainstLaterInternalListeners(t *testing.T) {
	for _, tc := range []struct {
		name           string
		route, metrics string
	}{
		{"route listener", "127.0.0.1:54619", "127.0.0.1:9303"},
		{"metrics listener", "127.0.0.1:1444", "127.0.0.1:54619"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := multiInstanceStore(t)
			now := time.Now()
			first := scopedReq("tenant-one", "ex-1", "ir-1", 8443, "127.0.0.1:1443", "127.0.0.1:9202", "127.0.0.1:9302")
			first.ClientPort = 54619
			second := scopedReq("tenant-two", "ex-2", "ir-1", 8443, tc.route, "127.0.0.1:9202", tc.metrics)
			if _, err := s.CreateTunnel(first, now); err != nil {
				t.Fatal(err)
			}
			if _, err := s.CreateTunnel(second, now); err == nil {
				t.Fatal("new internal listener stole a persisted client-port claim")
			}
		})
	}
}
