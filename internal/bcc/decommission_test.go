package bcc

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/tunnelnode"
)

func activateTunnel(t *testing.T, s *Store, now time.Time) Tunnel {
	t.Helper()
	tn := driveToObserve(t, s, now)
	ackObserved(t, s, "ir-1", goodObserved(tn, tunnelnode.RoleIR), now)
	ackObserved(t, s, "ex-1", goodObserved(tn, tunnelnode.RoleEX), now)
	for _, node := range []string{"ir-1", "ex-1"} {
		jobs := pullAll(t, s, node)
		if len(jobs) != 1 || jobs[0].Type != JobTunnelFinalize {
			t.Fatalf("%s finalize job: %+v", node, jobs)
		}
		if err := s.AckJob(node, "tok-"+node, jobs[0].ID, "succeeded", "finalized"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.AdvanceTunnels(now); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := s.GetTunnel(tn.ID)
	if got.Phase != TunnelActive {
		t.Fatalf("activation ended in %s", got.Phase)
	}
	return got
}

func retireMessage(tn Tunnel, node string) string {
	d := tn.Digests[node]
	gen := tn.ObservedGen[node]
	before := tunnelnode.Live{
		InstanceID: tn.InstanceID, ConfigPresent: true, UnitPresent: true,
		ConfigSHA256: d.Config, UnitSHA256: d.Unit, MarkerSHA256: d.Marker,
		MarkerPresent: true, MarkerManagedBy: "baft", MarkerTunnelID: tn.ID,
		MarkerGeneration: gen, MarkerConfigMatches: true, MarkerUnitMatches: true,
		NodeGeneration: gen, ServiceActive: true,
	}
	ev := tunnelnode.RetireEvidence{
		TunnelID: tn.ID, Instance: tn.InstanceID, Before: before,
		After: tunnelnode.Live{InstanceID: tn.InstanceID, NodeGeneration: gen},
	}
	b, _ := json.Marshal(ev)
	return string(b)
}

func TestDecommissionPlanIsReadOnlyAndApplyStartsWithIR(t *testing.T) {
	s := tunnelStore(t)
	now := time.Now().UTC()
	tn := activateTunnel(t, s, now)
	beforeJobs := len(s.ListJobs())
	plan, err := s.BuildDecommissionPlan(tn.ID, now)
	if err != nil || !plan.OK || plan.Hash == "" || !plan.Disruptive {
		t.Fatalf("plan %+v err=%v", plan, err)
	}
	if len(s.ListJobs()) != beforeJobs {
		t.Fatal("decommission planning mutated jobs")
	}
	if len(plan.Nodes) != 2 || plan.Nodes[0].Role != tunnelnode.RoleIR || plan.Nodes[1].Role != tunnelnode.RoleEX {
		t.Fatalf("plan node order %+v", plan.Nodes)
	}

	got, _, err := s.StartDecommissionFromPlan(tn.ID, plan.Hash, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Phase != TunnelDecommissioningIR {
		t.Fatalf("phase %s", got.Phase)
	}
	jobs := pullAll(t, s, "ir-1")
	if len(jobs) != 1 || jobs[0].Type != JobTunnelRetire {
		t.Fatalf("IR retire job %+v", jobs)
	}
	if jobs[0].Params["generation"] != "4" ||
		jobs[0].Params["config_sha256"] != strings.Repeat("a", 64) ||
		jobs[0].Params["unit_sha256"] != strings.Repeat("b", 64) ||
		jobs[0].Params["marker_sha256"] != strings.Repeat("c", 64) {
		t.Fatalf("retire guard params %+v", jobs[0].Params)
	}
	if ex := pullAll(t, s, "ex-1"); len(ex) != 0 {
		t.Fatalf("EX retire was queued before IR completed: %+v", ex)
	}
}

func TestDecommissionRequiresCurrentReviewedPlan(t *testing.T) {
	s := tunnelStore(t)
	now := time.Now().UTC()
	tn := activateTunnel(t, s, now)
	plan, err := s.BuildDecommissionPlan(tn.ID, now)
	if err != nil {
		t.Fatal(err)
	}

	s.mu.Lock()
	x := s.st.Tunnels[tn.ID]
	x.Digests["ir-1"] = NodeDigests{
		Config: strings.Repeat("d", 64), Unit: strings.Repeat("b", 64), Marker: strings.Repeat("c", 64),
	}
	s.st.Tunnels[tn.ID] = x
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()

	_, cur, err := s.StartDecommissionFromPlan(tn.ID, plan.Hash, now)
	var stale ErrStaleDecommissionPlan
	if !errors.As(err, &stale) || cur.Hash == "" || cur.Hash == plan.Hash {
		t.Fatalf("stale plan accepted or not refreshed: cur=%+v err=%v", cur, err)
	}
	if got, _ := s.GetTunnel(tn.ID); got.Phase != TunnelActive {
		t.Fatalf("stale plan mutated phase to %s", got.Phase)
	}
}

func TestDecommissionCompletesIRThenEX(t *testing.T) {
	s := tunnelStore(t)
	now := time.Now().UTC()
	tn := activateTunnel(t, s, now)
	plan, _ := s.BuildDecommissionPlan(tn.ID, now)
	if _, _, err := s.StartDecommissionFromPlan(tn.ID, plan.Hash, now); err != nil {
		t.Fatal(err)
	}

	ir := pullAll(t, s, "ir-1")
	if len(ir) != 1 {
		t.Fatalf("IR jobs %+v", ir)
	}
	if err := s.AckJobOutput("ir-1", "tok-ir-1", ir[0].ID, "succeeded", retireMessage(tn, "ir-1"), ""); err != nil {
		t.Fatal(err)
	}
	if ev, err := s.AdvanceTunnels(now); err != nil || len(ev) != 0 {
		t.Fatalf("IR advance events=%+v err=%v", ev, err)
	}
	mid, _ := s.GetTunnel(tn.ID)
	if mid.Phase != TunnelDecommissioningEX || !mid.DecommissionedNodes["ir-1"] || mid.DecommissionedNodes["ex-1"] {
		t.Fatalf("mid state %+v", mid)
	}

	ex := pullAll(t, s, "ex-1")
	if len(ex) != 1 || ex[0].Type != JobTunnelRetire {
		t.Fatalf("EX retire job %+v", ex)
	}
	if err := s.AckJobOutput("ex-1", "tok-ex-1", ex[0].ID, "succeeded", retireMessage(tn, "ex-1"), ""); err != nil {
		t.Fatal(err)
	}
	events, err := s.AdvanceTunnels(now)
	if err != nil || len(events) != 1 || events[0].Action != "tunnel.decommissioned" {
		t.Fatalf("final events %+v err=%v", events, err)
	}
	done, _ := s.GetTunnel(tn.ID)
	if done.Phase != TunnelDecommissioned || !done.DecommissionedNodes["ir-1"] || !done.DecommissionedNodes["ex-1"] {
		t.Fatalf("final state %+v", done)
	}
}

func TestDecommissionPartialFailureRetriesOnlyPendingEX(t *testing.T) {
	s := tunnelStore(t)
	now := time.Now().UTC()
	tn := activateTunnel(t, s, now)
	plan, _ := s.BuildDecommissionPlan(tn.ID, now)
	s.StartDecommissionFromPlan(tn.ID, plan.Hash, now)

	ir := pullAll(t, s, "ir-1")[0]
	s.AckJobOutput("ir-1", "tok-ir-1", ir.ID, "succeeded", retireMessage(tn, "ir-1"), "")
	s.AdvanceTunnels(now)
	ex := pullAll(t, s, "ex-1")[0]
	s.AckJob("ex-1", "tok-ex-1", ex.ID, "failed", "temporary node error")
	events, _ := s.AdvanceTunnels(now)
	if len(events) != 1 || events[0].Action != "tunnel.decommission_failed" {
		t.Fatalf("failure events %+v", events)
	}
	failed, _ := s.GetTunnel(tn.ID)
	if failed.Phase != TunnelDecommissionFailed || !failed.DecommissionedNodes["ir-1"] {
		t.Fatalf("failed state %+v", failed)
	}

	retryPlan, err := s.BuildDecommissionPlan(tn.ID, now.Add(time.Minute))
	if err != nil || !retryPlan.OK || !retryPlan.Nodes[0].Already {
		t.Fatalf("retry plan %+v err=%v", retryPlan, err)
	}
	retry, _, err := s.StartDecommissionFromPlan(tn.ID, retryPlan.Hash, now.Add(time.Minute))
	if err != nil || retry.Phase != TunnelDecommissioningEX {
		t.Fatalf("retry %+v err=%v", retry, err)
	}
	if irAgain := pullAll(t, s, "ir-1"); len(irAgain) != 0 {
		t.Fatalf("retry touched already retired IR: %+v", irAgain)
	}
	exRetry := pullAll(t, s, "ex-1")
	if len(exRetry) != 1 || exRetry[0].Type != JobTunnelRetire {
		t.Fatalf("retry EX job %+v", exRetry)
	}
	s.AckJobOutput("ex-1", "tok-ex-1", exRetry[0].ID, "succeeded", retireMessage(tn, "ex-1"), "")
	s.AdvanceTunnels(now.Add(time.Minute))
	if done, _ := s.GetTunnel(tn.ID); done.Phase != TunnelDecommissioned {
		t.Fatalf("retry ended in %s", done.Phase)
	}
}

func TestDecommissionStateSurvivesBCCRestart(t *testing.T) {
	s := tunnelStore(t)
	now := time.Now().UTC()
	tn := activateTunnel(t, s, now)
	plan, _ := s.BuildDecommissionPlan(tn.ID, now)
	started, _, err := s.StartDecommissionFromPlan(tn.ID, plan.Hash, now)
	if err != nil {
		t.Fatal(err)
	}
	path := s.path
	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := reopened.GetTunnel(tn.ID)
	if !ok || got.Phase != TunnelDecommissioningIR || got.JobID != started.JobID || got.DecommissionPlanHash != plan.Hash {
		t.Fatalf("restart state %+v", got)
	}
	jobs := pullAll(t, reopened, "ir-1")
	if len(jobs) != 1 || jobs[0].ID != started.JobID || jobs[0].Type != JobTunnelRetire {
		t.Fatalf("restart lost retire job %+v", jobs)
	}
}

func TestStaleAgentBlocksDecommissionPlan(t *testing.T) {
	s := tunnelStore(t)
	now := time.Now().UTC()
	tn := activateTunnel(t, s, now)
	s.mu.Lock()
	n := s.st.Nodes["ir-1"]
	n.AgentSeen = now.Add(-agentFreshness - time.Second)
	s.st.Nodes["ir-1"] = n
	s.mu.Unlock()
	plan, err := s.BuildDecommissionPlan(tn.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if plan.OK {
		t.Fatalf("stale agent passed decommission plan: %+v", plan.Gates)
	}
	found := false
	for _, g := range plan.Gates {
		if g.Name == "node: ir-1" && g.Status == "FAIL" && strings.Contains(g.Detail, "stale") {
			found = true
		}
	}
	if !found {
		t.Fatalf("stale-agent gate missing: %+v", plan.Gates)
	}
}

func TestKnownDriftBlocksDecommissionPlan(t *testing.T) {
	s := tunnelStore(t)
	now := time.Now().UTC()
	tn := activateTunnel(t, s, now)
	s.mu.Lock()
	x := s.st.Tunnels[tn.ID]
	x.Drift = &DriftReport{State: DriftDrifted, CheckedAt: now, Nodes: map[string]DriftNode{
		"ir-1": {State: DriftDrifted, Problems: []string{"config changed"}},
	}}
	s.st.Tunnels[tn.ID] = x
	s.mu.Unlock()
	plan, err := s.BuildDecommissionPlan(tn.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if plan.OK {
		t.Fatalf("known drift passed plan: %+v", plan.Gates)
	}
}

func TestDecommissionFailedBlocksReplacementInSameManagedSlot(t *testing.T) {
	s := tunnelStore(t)
	now := time.Now().UTC()
	tn := activateTunnel(t, s, now)
	s.mu.Lock()
	x := s.st.Tunnels[tn.ID]
	x.Phase = TunnelDecommissionFailed
	x.DecommissionedNodes = map[string]bool{"ir-1": true}
	s.st.Tunnels[tn.ID] = x
	s.mu.Unlock()
	if _, err := s.CreateTunnel(TunnelRequest{EXNode: "ex-1", IRNode: "ir-1"}, now.Add(time.Minute)); err == nil || !strings.Contains(err.Error(), "incomplete decommission") {
		t.Fatalf("replacement allowed over partial decommission: %v", err)
	}
}

func TestDecommissionRequestAndOutcomeHaveDurableAuditIntents(t *testing.T) {
	s := tunnelStore(t)
	now := time.Now().UTC()
	tn := activateTunnel(t, s, now)
	plan, err := s.BuildDecommissionPlan(tn.ID, now)
	if err != nil || !plan.OK {
		t.Fatalf("plan %+v err=%v", plan, err)
	}
	started, _, err := s.StartDecommissionFromPlanAudited(tn.ID, plan.Hash, now, AuditEntry{Actor: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	pending := s.PendingSecurityAuditIntents()
	if len(pending) != 1 || pending[0].Action != "tunnel.decommission.request" || pending[0].Target != tn.ID {
		t.Fatalf("request audit intent %+v", pending)
	}
	ir := pullAll(t, s, "ir-1")
	if len(ir) != 1 || ir[0].ID != started.JobID {
		t.Fatalf("IR retire jobs %+v", ir)
	}
	if err := s.AckJobOutput("ir-1", "tok-ir-1", ir[0].ID, "succeeded", retireMessage(tn, "ir-1"), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdvanceTunnels(now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	ex := pullAll(t, s, "ex-1")
	if len(ex) != 1 {
		t.Fatalf("EX retire jobs %+v", ex)
	}
	if err := s.AckJobOutput("ex-1", "tok-ex-1", ex[0].ID, "succeeded", retireMessage(tn, "ex-1"), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdvanceTunnels(now.Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	pending = s.PendingSecurityAuditIntents()
	var request, outcome bool
	for _, a := range pending {
		request = request || a.Action == "tunnel.decommission.request"
		outcome = outcome || a.Action == "tunnel.decommissioned"
	}
	if !request || !outcome {
		t.Fatalf("durable audit intents missing request=%v outcome=%v: %+v", request, outcome, pending)
	}

	reopened, err := OpenStore(s.path)
	if err != nil {
		t.Fatal(err)
	}
	pending = reopened.PendingSecurityAuditIntents()
	request, outcome = false, false
	for _, a := range pending {
		request = request || a.Action == "tunnel.decommission.request"
		outcome = outcome || a.Action == "tunnel.decommissioned"
	}
	if !request || !outcome {
		t.Fatalf("restart lost audit intents: %+v", pending)
	}
}

func TestDecommissionAPIPlanApplyStatusAndAudit(t *testing.T) {
	s := tunnelStore(t)
	now := time.Now().UTC()
	tn := activateTunnel(t, s, now)
	app, err := NewServer(s, "admin")
	if err != nil {
		t.Fatal(err)
	}

	planRR := httptest.NewRecorder()
	app.Handler().ServeHTTP(planRR, authReq(http.MethodPost, "/api/tunnels/decommission/plan", "admin", map[string]string{"id": tn.ID}))
	if planRR.Code != http.StatusOK {
		t.Fatalf("plan status=%d body=%s", planRR.Code, planRR.Body.String())
	}
	var plan DecommissionPlan
	if err := json.Unmarshal(planRR.Body.Bytes(), &plan); err != nil || !plan.OK || plan.Hash == "" {
		t.Fatalf("plan %+v err=%v", plan, err)
	}

	applyRR := httptest.NewRecorder()
	app.Handler().ServeHTTP(applyRR, authReq(http.MethodPost, "/api/tunnels/decommission", "admin", map[string]string{"id": tn.ID, "plan_hash": plan.Hash}))
	if applyRR.Code != http.StatusAccepted {
		t.Fatalf("apply status=%d body=%s", applyRR.Code, applyRR.Body.String())
	}
	var started Tunnel
	if err := json.Unmarshal(applyRR.Body.Bytes(), &started); err != nil || started.Phase != TunnelDecommissioningIR {
		t.Fatalf("started %+v err=%v", started, err)
	}

	ir := pullAll(t, s, "ir-1")
	if len(ir) != 1 {
		t.Fatalf("IR jobs %+v", ir)
	}
	if err := s.AckJobOutput("ir-1", "tok-ir-1", ir[0].ID, "succeeded", retireMessage(tn, "ir-1"), ""); err != nil {
		t.Fatal(err)
	}
	app.AdvanceTunnels()

	ex := pullAll(t, s, "ex-1")
	if len(ex) != 1 {
		t.Fatalf("EX jobs %+v", ex)
	}
	if err := s.AckJobOutput("ex-1", "tok-ex-1", ex[0].ID, "succeeded", retireMessage(tn, "ex-1"), ""); err != nil {
		t.Fatal(err)
	}
	app.AdvanceTunnels()

	statusRR := httptest.NewRecorder()
	app.Handler().ServeHTTP(statusRR, authReq(http.MethodGet, "/api/tunnels?id="+tn.ID, "admin", nil))
	if statusRR.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", statusRR.Code, statusRR.Body.String())
	}
	var done Tunnel
	if err := json.Unmarshal(statusRR.Body.Bytes(), &done); err != nil || done.Phase != TunnelDecommissioned {
		t.Fatalf("status tunnel %+v err=%v", done, err)
	}

	auditRR := httptest.NewRecorder()
	app.Handler().ServeHTTP(auditRR, authReq(http.MethodGet, "/api/audit", "admin", nil))
	if auditRR.Code != http.StatusOK {
		t.Fatalf("audit status=%d body=%s", auditRR.Code, auditRR.Body.String())
	}
	body := auditRR.Body.String()
	for _, want := range []string{"tunnel.decommission.plan", "tunnel.decommission.request", "tunnel.decommissioned"} {
		if !strings.Contains(body, want) {
			t.Errorf("audit lacks %s: %s", want, body)
		}
	}
}


func sharedTenantDecommissionFixture(t *testing.T) (*Store, Tunnel, Tunnel, Tunnel, time.Time) {
	t.Helper()
	now := time.Now().UTC()
	s := tunnelStore(t)
	owner := activateTunnel(t, s, now)
	for id, address := range map[string]string{
		"ir-2": "203.0.113.8:22",
		"ir-3": "203.0.113.9:22",
	} {
		if _, err := s.UpsertNode(Node{ID: id, Address: address, Role: "worker"}, "tok-"+id); err != nil {
			t.Fatal(err)
		}
	}
	digest := func(char string) NodeDigests {
		return NodeDigests{Config: strings.Repeat(char, 64), Unit: strings.Repeat(char, 64), Marker: strings.Repeat(char, 64)}
	}
	tenant := Tunnel{
		ID: "tun-shared-tenant-a", JoinOwnerID: owner.ID, InstanceID: owner.InstanceID,
		EXNode: owner.EXNode, IRNode: "ir-2", EXConfigNodeID: "ex-config", EXListen: "0.0.0.0:8443",
		Phase: TunnelActive, RouteID: "route-a", PeerID: "urn:baft:node:ir-2", PeerPublicKey: strings.Repeat("A", 43),
		Digests: map[string]NodeDigests{owner.EXNode: owner.Digests[owner.EXNode], "ir-2": digest("1")},
		ObservedGen: map[string]int{owner.EXNode: owner.ObservedGen[owner.EXNode], "ir-2": 1},
	}
	sibling := Tunnel{
		ID: "tun-shared-tenant-b", JoinOwnerID: owner.ID, InstanceID: owner.InstanceID,
		EXNode: owner.EXNode, IRNode: "ir-3", EXConfigNodeID: "ex-config", EXListen: "0.0.0.0:8443",
		Phase: TunnelActive, RouteID: "route-b", PeerID: "urn:baft:node:ir-3", PeerPublicKey: strings.Repeat("B", 43),
		Digests: map[string]NodeDigests{owner.EXNode: owner.Digests[owner.EXNode], "ir-3": digest("2")},
		ObservedGen: map[string]int{owner.EXNode: owner.ObservedGen[owner.EXNode], "ir-3": 1},
	}
	s.mu.Lock()
	for _, id := range []string{owner.EXNode, owner.IRNode, tenant.IRNode, sibling.IRNode} {
		n := s.st.Nodes[id]
		n.AgentSeen = now
		s.st.Nodes[id] = n
	}
	s.st.Tunnels[tenant.ID] = tenant
	s.st.Tunnels[sibling.ID] = sibling
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()
	return s, owner, tenant, sibling, now
}

func TestJoinedTenantDecommissionReviewedPlanRemovesOnlyTenant(t *testing.T) {
	s, owner, tenant, sibling, now := sharedTenantDecommissionFixture(t)
	plan, err := s.BuildDecommissionPlan(tenant.ID, now)
	if err != nil || !plan.OK {
		t.Fatalf("active shared owner and sibling should be allowed: plan=%+v err=%v", plan, err)
	}
	started, _, err := s.StartDecommissionFromPlan(tenant.ID, plan.Hash, now)
	if err != nil || started.Phase != TunnelDecommissioningIR {
		t.Fatalf("reviewed decommission did not start with the tenant IR: tunnel=%+v err=%v", started, err)
	}
	irJobs := pullAll(t, s, tenant.IRNode)
	if len(irJobs) != 1 || irJobs[0].Type != JobTunnelRetire {
		t.Fatalf("tenant IR retire job: %+v", irJobs)
	}
	if err := s.AckJobOutput(tenant.IRNode, "tok-"+tenant.IRNode, irJobs[0].ID, "succeeded", retireMessage(tenant, tenant.IRNode), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdvanceTunnels(now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	exJobs := pullAll(t, s, tenant.EXNode)
	if len(exJobs) != 1 || exJobs[0].Type != agentjob.ActionTunnelTenantRemove ||
		exJobs[0].Params["owner_tunnel_id"] != owner.ID || exJobs[0].Params["peer_id"] != tenant.PeerID {
		t.Fatalf("reviewed tenant decommission must remove only the selected tenant: %+v", exJobs)
	}
	before := tenant.Digests[tenant.EXNode]
	ev := tunnelnode.TenantMutationEvidence{
		ID: exJobs[0].Params["mutation_id"], Operation: "remove", NodeID: tenant.EXConfigNodeID,
		OwnerTunnelID: owner.ID, Generation: tenant.ObservedGen[tenant.EXNode] + 1,
		BeforeConfigSHA: before.Config, AfterConfigSHA: strings.Repeat("d", 64),
		BeforeUnitSHA: before.Unit, AfterUnitSHA: strings.Repeat("e", 64),
		BeforeMarkerSHA: before.Marker, AfterMarkerSHA: strings.Repeat("f", 64),
	}
	output, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AckJobOutput(tenant.EXNode, "tok-"+tenant.EXNode, exJobs[0].ID, "succeeded", "tenant removed", string(output)); err != nil {
		t.Fatal(err)
	}
	events, err := s.AdvanceTunnels(now.Add(2 * time.Second))
	if err != nil || len(events) != 1 || events[0].Action != "tunnel.decommissioned" {
		t.Fatalf("tenant decommission events=%+v err=%v", events, err)
	}
	removed, _ := s.GetTunnel(tenant.ID)
	ownerAfter, _ := s.GetTunnel(owner.ID)
	siblingAfter, _ := s.GetTunnel(sibling.ID)
	if removed.Phase != TunnelDecommissioned || ownerAfter.Phase != TunnelActive || siblingAfter.Phase != TunnelActive {
		t.Fatalf("shared listener lifecycle changed unexpectedly: tenant=%s owner=%s sibling=%s",
			removed.Phase, ownerAfter.Phase, siblingAfter.Phase)
	}
	for _, got := range []Tunnel{ownerAfter, siblingAfter} {
		if got.Digests[owner.EXNode].Config != ev.AfterConfigSHA ||
			got.Digests[owner.EXNode].Unit != ev.AfterUnitSHA ||
			got.Digests[owner.EXNode].Marker != ev.AfterMarkerSHA ||
			got.ObservedGen[owner.EXNode] != ev.Generation {
			t.Fatalf("shared owner/sibling baseline did not advance: %+v", got)
		}
	}
}

func TestJoinedTenantDecommissionBlocksUncertainSharedState(t *testing.T) {
	tests := map[string]func(*Store, Tunnel, Tunnel, Tunnel){
		"owner rollback failed": func(s *Store, owner, _, _ Tunnel) {
			owner.Phase = TunnelRollbackFailed
			s.st.Tunnels[owner.ID] = owner
		},
		"owner decommission failed": func(s *Store, owner, _, _ Tunnel) {
			owner.Phase = TunnelDecommissionFailed
			s.st.Tunnels[owner.ID] = owner
		},
		"sibling rollback failed": func(s *Store, _, _, sibling Tunnel) {
			sibling.Phase = TunnelRollbackFailed
			s.st.Tunnels[sibling.ID] = sibling
		},
		"sibling decommission failed": func(s *Store, _, _, sibling Tunnel) {
			sibling.Phase = TunnelDecommissionFailed
			s.st.Tunnels[sibling.ID] = sibling
		},
		"owner drift check running": func(s *Store, owner, _, _ Tunnel) {
			owner.DriftJobs = []string{"drift-owner"}
			s.st.Tunnels[owner.ID] = owner
		},
		"owner drifted": func(s *Store, owner, _, _ Tunnel) {
			owner.Drift = &DriftReport{State: DriftDrifted}
			s.st.Tunnels[owner.ID] = owner
		},
		"sibling drift check running": func(s *Store, _, _, sibling Tunnel) {
			sibling.DriftJobs = []string{"drift-sibling"}
			s.st.Tunnels[sibling.ID] = sibling
		},
		"sibling drifted": func(s *Store, _, _, sibling Tunnel) {
			sibling.Drift = &DriftReport{State: DriftDrifted}
			s.st.Tunnels[sibling.ID] = sibling
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			s, _, tenant, sibling, now := sharedTenantDecommissionFixture(t)
			s.mu.Lock()
			owner := s.st.Tunnels[tenant.JoinOwnerID]
			sibling = s.st.Tunnels[sibling.ID]
			mutate(s, owner, tenant, sibling)
			s.mu.Unlock()
			plan, err := s.BuildDecommissionPlan(tenant.ID, now)
			if err != nil {
				t.Fatal(err)
			}
			if plan.OK {
				t.Fatalf("uncertain shared owner/sibling state passed the decommission plan: %+v", plan.Gates)
			}
			found := false
			for _, gate := range plan.Gates {
				if gate.Name == "conflicting tunnel change" && gate.Status == "FAIL" {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing shared-state failure gate: %+v", plan.Gates)
			}
		})
	}
}

func TestJoinedTenantDecommissionBlocksOwnerWidePendingMutation(t *testing.T) {
	for _, op := range []string{agentjob.ActionTunnelTenantAdd, agentjob.ActionTunnelTenantRemove} {
		for _, status := range []string{"queued", "dispatched"} {
			for _, target := range []string{"owner", "sibling"} {
				t.Run(op+"/"+status+"/"+target, func(t *testing.T) {
					s, owner, tenant, sibling, now := sharedTenantDecommissionFixture(t)
					targetID := owner.ID
					if target == "sibling" {
						targetID = sibling.ID
					}
					s.mu.Lock()
					if s.st.Jobs == nil {
						s.st.Jobs = map[string]Job{}
					}
					s.st.Jobs["pending-tenant-mutation"] = Job{
						ID: "pending-tenant-mutation", Type: op, Status: status, NodeID: owner.EXNode,
						Params: map[string]string{"tunnel_id": targetID, "owner_tunnel_id": owner.ID},
					}
					s.mu.Unlock()
					plan, err := s.BuildDecommissionPlan(tenant.ID, now)
					if err != nil {
						t.Fatal(err)
					}
					if plan.OK {
						t.Fatalf("pending owner-wide %s job in %s state passed: %+v", op, status, plan.Gates)
					}
					found := false
					for _, gate := range plan.Gates {
						if gate.Name == "conflicting tunnel change" && gate.Status == "FAIL" && strings.Contains(gate.Detail, "pending tenant mutation") {
							found = true
						}
					}
					if !found {
						t.Fatalf("missing pending-mutation gate: %+v", plan.Gates)
					}
				})
			}
		}
	}
}
