package bcc

import (
	"bytes"
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

func TestHQReviewedClientPortCannotAliasOwnIRListeners(t *testing.T) {
	for _, tc := range []struct {
		name   string
		client int
		req    TunnelRequest
	}{
		{"explicit IR metrics", 9302, scopedReq("hq-own-metrics", "ex-1", "ir-1", 8443, "127.0.0.1:1443", "127.0.0.1:9202", "127.0.0.1:9302")},
		{"IR route", 1443, scopedReq("hq-own-route", "ex-1", "ir-1", 8443, "127.0.0.1:1443", "127.0.0.1:9202", "127.0.0.1:9302")},
		{"legacy default IR metrics", 9191, TunnelRequest{EXNode: "ex-1", IRNode: "ir-1", RouteListen: "127.0.0.1:1443"}},
		{"legacy default IR route", 1443, TunnelRequest{EXNode: "ex-1", IRNode: "ir-1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := multiInstanceStore(t)
			req := tc.req
			req.ClientPort = tc.client
			now := time.Now()
			before := stateJSONForTest(t, s)
			if _, err := s.BuildPlan(req, now); err == nil {
				t.Fatal("reviewed plan accepted self-aliasing IR ingress")
			}
			if _, err := s.CreateTunnel(req, now); err == nil {
				t.Fatal("CreateTunnel accepted self-aliasing IR ingress")
			}
			if got := stateJSONForTest(t, s); !bytes.Equal(before, got) {
				t.Fatal("rejected candidate mutated durable state/jobs")
			}
		})
	}
}

func TestHQReviewedClientPortBlocksHistoricalCrossRoleIRToEX(t *testing.T) {
	for _, tc := range []struct {
		name    string
		port    int
		metrics string
	}{
		{"EX transport", 54619, "127.0.0.1:9212"},
		{"EX metrics", 8444, "127.0.0.1:54619"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := multiInstanceStore(t)
			s.mu.Lock()
			if s.st.Tunnels == nil {
				s.st.Tunnels = map[string]Tunnel{}
			}
			s.st.Tunnels["old-ir"] = Tunnel{ID: "old-ir", InstanceID: "old", IRNode: "ex-1", EXNode: "ex-2", ClientPort: 54619, RouteListen: "127.0.0.1:1449", IRMetricsListen: "127.0.0.1:9319", Port: 8449, EXMetricsListen: "127.0.0.1:9219", Phase: TunnelActive}
			s.mu.Unlock()
			req := scopedReq("cross-role-new", "ex-1", "ir-2", tc.port, "127.0.0.1:1445", tc.metrics, "127.0.0.1:9305")
			before := stateJSONForTest(t, s)
			if _, err := s.BuildPlan(req, time.Now()); err == nil {
				t.Fatal("plan ignores existing IR client claim when new role is EX")
			}
			if _, err := s.CreateTunnel(req, time.Now()); err == nil {
				t.Fatal("create steals historical IR client port from EX role")
			}
			if got := stateJSONForTest(t, s); !bytes.Equal(before, got) {
				t.Fatal("blocked cross-role operation changed state")
			}
		})
	}
}

func TestHQReviewedClientPortAllowsExactManagedSlotReplacement(t *testing.T) {
	s := multiInstanceStore(t)
	req := scopedReq("slot-owner", "ex-1", "ir-1", 8443, "127.0.0.1:1443", "127.0.0.1:9202", "127.0.0.1:9302")
	req.ClientPort = 54619
	first, err := s.CreateTunnel(req, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	saved := s.st.Tunnels[first.ID]
	saved.Phase = TunnelActive
	s.st.Tunnels[first.ID] = saved
	s.mu.Unlock()
	if _, err := s.BuildPlan(req, time.Now()); err != nil {
		t.Fatalf("same-slot replacement falsely rejected: %v", err)
	}
}
