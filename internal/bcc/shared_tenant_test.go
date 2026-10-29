package bcc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/tunnelnode"
)

func TestSharedTenantMutationJobUsesAcceptedDigestsAndReconcilesReceipt(t *testing.T) {
	s := tunnelStore(t)
	now := time.Now().UTC()
	tn := newTunnel(t, s, now)
	if len(tn.Jobs) == 0 {
		t.Fatal("expected initial tunnel job in fixture")
	}
	if err := s.AckJob("ex-1", "tok-ex-1", tn.Jobs[0], "failed", "fixture job retired"); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	active, ok := s.st.Tunnels[tn.ID]
	if !ok {
		s.mu.Unlock()
		t.Fatal("tunnel missing")
	}
	active.Phase = TunnelActive
	active.Digests = map[string]NodeDigests{
		active.EXNode: {Config: strings.Repeat("a", 64), Unit: strings.Repeat("b", 64), Marker: strings.Repeat("c", 64)},
	}
	active.ObservedGen = map[string]int{active.EXNode: 2}
	s.st.Tunnels[tn.ID] = active
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()

	req := SharedTenantMutationRequest{
		TunnelID: tn.ID, Operation: "add", NodeID: "ex-config-id", Listen: "0.0.0.0:443",
		PeerID: "urn:baft:node:ir-c", PeerPublicKey: strings.Repeat("A", 43),
		RouteID: "port-443", Target: "127.0.0.1:9003",
	}
	job, err := s.CreateSharedTenantMutationJob(req, now)
	if err != nil {
		t.Fatalf("create tenant mutation job: %v", err)
	}
	if job.Type != agentjob.ActionTunnelTenantAdd ||
		job.Params["owner_tunnel_id"] != tn.ID || job.Params["config_sha256"] != strings.Repeat("a", 64) ||
		job.Params["unit_sha256"] != strings.Repeat("b", 64) || job.Params["marker_sha256"] != strings.Repeat("c", 64) ||
		job.Params["target"] != req.Target {
		t.Fatalf("job was not bound to accepted evidence: %+v", job)
	}
	dispatched := pullAll(t, s, "ex-1")
	if len(dispatched) != 1 || dispatched[0].ID != job.ID {
		t.Fatalf("expected tenant mutation to be dispatched: %+v", dispatched)
	}
	ev := tunnelnode.TenantMutationEvidence{
		ID: job.Params["mutation_id"], Operation: "add", NodeID: req.NodeID, OwnerTunnelID: tn.ID, Generation: 3,
		BeforeConfigSHA: strings.Repeat("a", 64), AfterConfigSHA: strings.Repeat("d", 64),
		BeforeUnitSHA: strings.Repeat("b", 64), AfterUnitSHA: strings.Repeat("f", 64),
		BeforeMarkerSHA: strings.Repeat("c", 64), AfterMarkerSHA: strings.Repeat("e", 64),
	}
	output, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AckJobOutput("ex-1", "tok-ex-1", job.ID, "succeeded", "tenant mutation committed", string(output)); err != nil {
		t.Fatal(err)
	}
	if err := s.ReconcileSharedTenantMutationAcks(now.Add(time.Second)); err != nil {
		t.Fatalf("reconcile tenant mutation receipt: %v", err)
	}
	got, ok := s.GetTunnel(tn.ID)
	if !ok || got.Digests["ex-1"].Config != strings.Repeat("d", 64) ||
		got.Digests["ex-1"].Marker != strings.Repeat("e", 64) || got.Digests["ex-1"].Unit != strings.Repeat("b", 64) ||
		got.ObservedGen["ex-1"] != 3 {
		t.Fatalf("BCC did not advance its accepted tenant baseline: %+v", got)
	}
	if err := s.ReconcileSharedTenantMutationAcks(now.Add(2 * time.Second)); err != nil {
		t.Fatalf("receipt reconciliation must be idempotent: %v", err)
	}
}

func TestSharedTenantMutationRejectsMissingAcceptedDigests(t *testing.T) {
	s := tunnelStore(t)
	now := time.Now().UTC()
	tn := newTunnel(t, s, now)
	s.mu.Lock()
	active := s.st.Tunnels[tn.ID]
	active.Phase = TunnelActive
	s.st.Tunnels[tn.ID] = active
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()
	_, err := s.CreateSharedTenantMutationJob(SharedTenantMutationRequest{
		TunnelID: tn.ID, Operation: "remove", NodeID: "ex-config-id", Listen: "0.0.0.0:443",
		PeerID: "urn:baft:node:ir-c", PeerPublicKey: strings.Repeat("A", 43), RouteID: "port-443",
	}, now)
	if err == nil {
		t.Fatal("must refuse tenant mutation without BCC-accepted ownership digests")
	}
}


func TestSharedTenantMutationHTTPQueuesAuditedJob(t *testing.T) {
	s := tunnelStore(t)
	now := time.Now().UTC()
	tn := newTunnel(t, s, now)
	if err := s.AckJob("ex-1", "tok-ex-1", tn.Jobs[0], "failed", "fixture job retired"); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	active := s.st.Tunnels[tn.ID]
	active.Phase = TunnelActive
	active.Digests = map[string]NodeDigests{
		active.EXNode: {Config: strings.Repeat("a", 64), Unit: strings.Repeat("b", 64), Marker: strings.Repeat("c", 64)},
	}
	active.ObservedGen = map[string]int{active.EXNode: 4}
	s.st.Tunnels[tn.ID] = active
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()

	srv, err := NewServer(s, "admin")
	if err != nil {
		t.Fatal(err)
	}
	req := authReq(http.MethodPost, "/api/tunnels/tenant", "admin", SharedTenantMutationRequest{
		TunnelID: tn.ID, Operation: "add", NodeID: "ex-config-id", Listen: "0.0.0.0:443",
		PeerID: "urn:baft:node:ir-new", PeerPublicKey: strings.Repeat("A", 43),
		RouteID: "port-443", Target: "127.0.0.1:9003",
	})
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("tenant API status=%d body=%s", rr.Code, rr.Body.String())
	}
	jobs := pullAll(t, s, "ex-1")
	if len(jobs) != 1 || jobs[0].Type != agentjob.ActionTunnelTenantAdd ||
		jobs[0].Params["owner_tunnel_id"] != tn.ID || jobs[0].Params["config_sha256"] != strings.Repeat("a", 64) ||
		jobs[0].Params["unit_sha256"] != strings.Repeat("b", 64) ||
		jobs[0].Params["marker_sha256"] != strings.Repeat("c", 64) {
		t.Fatalf("HTTP endpoint did not queue a digest-bound tenant action: %+v", jobs)
	}
}
