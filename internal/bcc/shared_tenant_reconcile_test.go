package bcc

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/tunnelnode"
)

// retireTenantThroughDecommission runs the reviewed decommission of a managed
// tenant to the end, the way the LAB does, and returns the owner afterwards.
func retireTenantThroughDecommission(t *testing.T, s *Store, owner, tenant Tunnel, now time.Time) Tunnel {
	t.Helper()
	plan, err := s.BuildDecommissionPlan(tenant.ID, now)
	if err != nil || !plan.OK {
		t.Fatalf("tenant decommission plan: %+v %v", plan, err)
	}
	if _, _, err := s.StartDecommissionFromPlan(tenant.ID, plan.Hash, now); err != nil {
		t.Fatal(err)
	}
	ir := pullAll(t, s, tenant.IRNode)
	if err := s.AckJobOutput(tenant.IRNode, "tok-"+tenant.IRNode, ir[0].ID, "succeeded", retireMessage(tenant, tenant.IRNode), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdvanceTunnels(now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	ex := pullAll(t, s, tenant.EXNode)
	if len(ex) != 1 || ex[0].Type != agentjob.ActionTunnelTenantRemove {
		t.Fatalf("tenant remove job: %+v", ex)
	}
	before := tenant.Digests[tenant.EXNode]
	ev := tunnelnode.TenantMutationEvidence{
		ID: ex[0].Params["mutation_id"], Operation: "remove", NodeID: tenant.EXConfigNodeID,
		OwnerTunnelID: owner.ID, Generation: tenant.ObservedGen[tenant.EXNode] + 1,
		BeforeConfigSHA: before.Config, AfterConfigSHA: strings.Repeat("d", 64),
		BeforeUnitSHA: before.Unit, AfterUnitSHA: strings.Repeat("e", 64),
		BeforeMarkerSHA: before.Marker, AfterMarkerSHA: strings.Repeat("f", 64),
	}
	out, _ := json.Marshal(ev)
	if err := s.AckJobOutput(tenant.EXNode, "tok-"+tenant.EXNode, ex[0].ID, "succeeded", "tenant removed", string(out)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdvanceTunnels(now.Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetTunnel(tenant.ID); got.Phase != TunnelDecommissioned {
		t.Fatalf("tenant ended as %s", got.Phase)
	}
	got, _ := s.GetTunnel(owner.ID)
	return got
}

// liveFor reports exactly what BCC holds for the node, as a healthy agent would.
func liveFor(tn Tunnel, node, role string) tunnelnode.Live {
	l := goodLive(tn, role)
	d, gen := tn.Digests[node], tn.ObservedGen[node]
	l.ConfigSHA256, l.UnitSHA256, l.MarkerSHA256 = d.Config, d.Unit, d.Marker
	l.MarkerGeneration, l.NodeGeneration = gen, gen
	return l
}

// The owner's reviewed decommission needs a fresh IN_SYNC drift check. After a
// managed tenant was retired, BCC's background loop must still settle that
// check (found by the real-hosts self-test: the adopted tenant_remove receipt
// stopped every tunnel's state machine, so the owner's drift jobs stayed
// pending forever and its decommission was refused).
func TestOwnerDriftSettlesAfterManagedTenantRetired(t *testing.T) {
	s, owner, tenant, sibling, now := sharedTenantDecommissionFixture(t)
	s.mu.Lock()
	sib := s.st.Tunnels[sibling.ID]
	sib.Phase = TunnelDecommissioned // one tenant only, as in the LAB
	s.st.Tunnels[sibling.ID] = sib
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()
	owner = retireTenantThroughDecommission(t, s, owner, tenant, now)

	if err := s.ReconcileSharedTenantMutationAcks(now.Add(3 * time.Second)); err != nil {
		t.Fatalf("an adopted tenant receipt must be history, not an error: %v", err)
	}
	app, err := NewServer(s, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartDrift(owner.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	for node, role := range map[string]string{owner.IRNode: tunnelnode.RoleIR, owner.EXNode: tunnelnode.RoleEX} {
		jobs := pullAll(t, s, node)
		if len(jobs) != 1 || jobs[0].Type != JobTunnelInspect {
			t.Fatalf("%s inspect job: %+v", node, jobs)
		}
		b, _ := json.Marshal(liveFor(owner, node, role))
		if err := s.AckJob(node, "tok-"+node, jobs[0].ID, "succeeded", string(b)); err != nil {
			t.Fatal(err)
		}
	}
	app.AdvanceTunnels()
	got, _ := s.GetTunnel(owner.ID)
	if len(got.DriftJobs) != 0 || got.Drift == nil || got.Drift.State != DriftInSync {
		t.Fatalf("owner drift check did not settle after the tenant was retired: jobs=%v drift=%+v", got.DriftJobs, got.Drift)
	}
	plan, err := s.BuildDecommissionPlan(owner.ID, time.Now())
	if err != nil || !plan.OK {
		t.Fatalf("owner decommission must be possible once its tenant is gone: %+v %v", plan, err)
	}
}

// A maintenance receipt adopted earlier stays history after a later verified
// generation of the same listener (a managed tenant joining or leaving). It
// must not be re-validated against the newer baseline.
func TestAdoptedMaintenanceReceiptIsHistoryAfterLaterGeneration(t *testing.T) {
	s, owner, _, _, now := sharedTenantDecommissionFixture(t)
	job, err := s.CreateSharedTenantMutationJob(SharedTenantMutationRequest{
		TunnelID: owner.ID, Operation: "add", NodeID: "ex-config", Listen: "0.0.0.0:8443",
		PeerID: "urn:baft:node:untracked-a", PeerPublicKey: strings.Repeat("A", 43),
		RouteID: "route-untracked", Target: "127.0.0.1:9001",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	pullAll(t, s, owner.EXNode)
	d := owner.Digests[owner.EXNode]
	gen := owner.ObservedGen[owner.EXNode] + 1
	ev := tunnelnode.TenantMutationEvidence{
		ID: job.Params["mutation_id"], Operation: "add", NodeID: "ex-config", OwnerTunnelID: owner.ID, Generation: gen,
		BeforeConfigSHA: d.Config, AfterConfigSHA: strings.Repeat("7", 64),
		BeforeUnitSHA: d.Unit, AfterUnitSHA: strings.Repeat("8", 64),
		BeforeMarkerSHA: d.Marker, AfterMarkerSHA: strings.Repeat("9", 64),
	}
	out, _ := json.Marshal(ev)
	if err := s.AckJobOutput(owner.EXNode, "tok-"+owner.EXNode, job.ID, "succeeded", "committed", string(out)); err != nil {
		t.Fatal(err)
	}
	if err := s.ReconcileSharedTenantMutationAcks(now); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetTunnel(owner.ID); got.ObservedGen[owner.EXNode] != gen || got.Digests[owner.EXNode].Config != ev.AfterConfigSHA {
		t.Fatalf("receipt not adopted: %+v", got)
	}
	// A later verified generation moves the shared baseline. Also cover state
	// written before adoption cleared the receipt: put the Output back.
	s.mu.Lock()
	o := s.st.Tunnels[owner.ID]
	o.Digests[owner.EXNode] = NodeDigests{Config: strings.Repeat("4", 64), Unit: strings.Repeat("5", 64), Marker: strings.Repeat("6", 64)}
	o.ObservedGen[owner.EXNode] = gen + 1
	s.st.Tunnels[owner.ID] = o
	j := s.st.Jobs[job.ID]
	j.Output = string(out)
	s.st.Jobs[job.ID] = j
	s.mu.Unlock()
	if err := s.ReconcileSharedTenantMutationAcks(now.Add(time.Second)); err != nil {
		t.Fatalf("superseded receipt must be history: %v", err)
	}
	if got, _ := s.GetTunnel(owner.ID); got.ObservedGen[owner.EXNode] != gen+1 || got.Digests[owner.EXNode].Config != strings.Repeat("4", 64) {
		t.Fatalf("a superseded receipt rolled the baseline back: %+v", got)
	}
}

// A receipt BCC cannot adopt is reported once, and it never stops the other
// tunnels: the background loop keeps settling drift checks.
func TestUnadoptableTenantReceiptDoesNotStopTheTunnelLoop(t *testing.T) {
	s, owner, _, _, now := sharedTenantDecommissionFixture(t)
	s.mu.Lock()
	s.st.Jobs["job-bad-receipt"] = Job{ID: "job-bad-receipt", Type: agentjob.ActionTunnelTenantAdd, NodeID: owner.EXNode, Status: "succeeded",
		Output: "not a receipt", Params: map[string]string{"tunnel_id": owner.ID, "owner_tunnel_id": owner.ID}}
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()
	if err := s.ReconcileSharedTenantMutationAcks(now); err == nil {
		t.Fatal("an unreadable receipt for an active tunnel must be reported")
	}
	app, err := NewServer(s, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartDrift(owner.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	for node, role := range map[string]string{owner.IRNode: tunnelnode.RoleIR, owner.EXNode: tunnelnode.RoleEX} {
		jobs := pullAll(t, s, node)
		b, _ := json.Marshal(liveFor(owner, node, role))
		if err := s.AckJob(node, "tok-"+node, jobs[0].ID, "succeeded", string(b)); err != nil {
			t.Fatal(err)
		}
	}
	app.AdvanceTunnels()
	app.AdvanceTunnels()
	if got, _ := s.GetTunnel(owner.ID); len(got.DriftJobs) != 0 || got.Drift == nil {
		t.Fatalf("the tunnel loop stopped behind an unadoptable receipt: %+v", got.DriftJobs)
	}
	entries, err := app.audit.List(0)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if e.Action == "tunnel.shared_tenant.reconcile" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("reconcile failure audited %d times, want once per distinct error", n)
	}
}
