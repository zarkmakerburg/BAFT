package bcc

import (
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/tunnelnode"
)

// Changing an active tunnel is done by building a replacement in the same
// managed slot: the old tunnel stays active until the replacement is proven,
// and is marked superseded only then.

func driveRequestToObserve(t *testing.T, s *Store, req TunnelRequest, now time.Time) Tunnel {
	t.Helper()
	tn, err := s.CreateTunnel(req, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range []struct{ node, out string }{
		{"ex-1", testPairCode}, {"ir-1", testReplyCode}, {"ex-1", ""}, {"ir-1", ""}, {"ir-1", ""}, {"ex-1", ""},
	} {
		j := pullAll(t, s, st.node)
		if len(j) != 1 {
			t.Fatalf("%s: expected one job, got %+v", st.node, j)
		}
		if err := s.AckJobOutput(st.node, "tok-"+st.node, j[0].ID, "succeeded", "ok", st.out); err != nil {
			t.Fatal(err)
		}
		s.AdvanceTunnels(now)
		if st.out == testReplyCode {
			completeCandidateStages(t, s, tn.ID, now)
		}
	}
	got, _ := s.GetTunnel(tn.ID)
	if got.Phase != TunnelObservingIR {
		t.Fatalf("phase %s, want observing_ir", got.Phase)
	}
	return got
}

func observedAt(tn Tunnel, role string, gen int) tunnelnode.Observed {
	o := goodObserved(tn, role)
	o.Generation, o.PreviousGeneration, o.NodeGeneration, o.MarkerGeneration = gen, gen-1, gen, gen
	return o
}

func activate(t *testing.T, s *Store, tn Tunnel, gen int, now time.Time) Tunnel {
	t.Helper()
	ackObserved(t, s, "ir-1", observedAt(tn, tunnelnode.RoleIR, gen), now)
	ackObserved(t, s, "ex-1", observedAt(tn, tunnelnode.RoleEX, gen), now)
	for _, node := range []string{"ir-1", "ex-1"} {
		j := pullAll(t, s, node)
		if len(j) != 1 || j[0].Type != JobTunnelFinalize {
			t.Fatalf("%s: expected finalize, got %+v", node, j)
		}
		s.AckJob(node, "tok-"+node, j[0].ID, "succeeded", "finalized")
		s.AdvanceTunnels(now)
	}
	got, _ := s.GetTunnel(tn.ID)
	if got.Phase != TunnelActive {
		t.Fatalf("tunnel %s is %s (%s), want active", tn.ID, got.Phase, got.Error)
	}
	return got
}

func TestReplacementInSameSlotSupersedesOldTunnelOnlyWhenActive(t *testing.T) {
	s := tunnelStore(t)
	now := time.Now()
	old := activate(t, s, driveRequestToObserve(t, s, TunnelRequest{EXNode: "ex-1", IRNode: "ir-1"}, now), 4, now)

	edit := TunnelRequest{EXNode: "ex-1", IRNode: "ir-1", Port: 9443, Target: "127.0.0.1:3443"}
	repl := driveRequestToObserve(t, s, edit, now)
	if repl.Port != 9443 || repl.Target != "127.0.0.1:3443" {
		t.Fatalf("replacement did not take the edited values: %+v", repl)
	}
	if got, _ := s.GetTunnel(old.ID); got.Phase != TunnelActive {
		t.Fatalf("old tunnel is %s while its replacement is still being proven", got.Phase)
	}

	activate(t, s, repl, 5, now)
	if got, _ := s.GetTunnel(old.ID); got.Phase != TunnelSuperseded {
		t.Fatalf("old tunnel is %s after the replacement became active, want superseded", got.Phase)
	}
	for _, id := range []string{"ex-1", "ir-1"} {
		if n, _ := s.GetNode(id); n.AppliedGeneration != 5 {
			t.Errorf("%s applied generation %d, want 5", id, n.AppliedGeneration)
		}
	}
}

func TestFailedReplacementLeavesOldTunnelActive(t *testing.T) {
	s := tunnelStore(t)
	now := time.Now()
	old := activate(t, s, driveRequestToObserve(t, s, TunnelRequest{EXNode: "ex-1", IRNode: "ir-1"}, now), 4, now)

	repl := driveRequestToObserve(t, s, TunnelRequest{EXNode: "ex-1", IRNode: "ir-1", Port: 9443}, now)
	o := observedAt(repl, tunnelnode.RoleIR, 5)
	o.ServiceActive = false
	ackObserved(t, s, "ir-1", o, now)
	if got, _ := s.GetTunnel(repl.ID); got.Phase != TunnelRollingBack {
		t.Fatalf("replacement is %s, want rolling_back", got.Phase)
	}
	if got, _ := s.GetTunnel(old.ID); got.Phase != TunnelActive || got.Port != 8443 {
		t.Fatalf("old tunnel is %s on port %d after a failed replacement, want active on 8443", got.Phase, got.Port)
	}
	for _, id := range []string{"ex-1", "ir-1"} {
		if n, _ := s.GetNode(id); n.AppliedGeneration != 4 {
			t.Errorf("%s applied generation %d after a failed replacement, want 4", id, n.AppliedGeneration)
		}
	}
}

func TestSecondReplacementWaitsForTheFirst(t *testing.T) {
	s := tunnelStore(t)
	now := time.Now()
	activate(t, s, driveRequestToObserve(t, s, TunnelRequest{EXNode: "ex-1", IRNode: "ir-1"}, now), 4, now)
	if _, err := s.CreateTunnel(TunnelRequest{EXNode: "ex-1", IRNode: "ir-1", Port: 9443}, now); err != nil {
		t.Fatal(err)
	}
	_, err := s.CreateTunnel(TunnelRequest{EXNode: "ex-1", IRNode: "ir-1", Port: 10443}, now)
	if err == nil || !strings.Contains(err.Error(), "still being built") {
		t.Fatalf("a second concurrent replacement was accepted: %v", err)
	}
}

// The dashboard's Edit action sends the old tunnel's full request with only
// the edited fields changed; the reviewed plan names the tunnel it replaces
// and deploying it by hash keeps every other field.
func TestEditPlanNamesReplacedTunnelAndKeepsOtherFields(t *testing.T) {
	s := tunnelStore(t)
	now := time.Now()
	base := TunnelRequest{EXNode: "ex-1", IRNode: "ir-1", RouteID: "service-alt", RouteListen: "127.0.0.1:1444", Target: "127.0.0.1:2444"}
	old := activate(t, s, driveRequestToObserve(t, s, base, now), 4, now)

	edit := TunnelRequest{InstanceID: old.InstanceID, EXNode: old.EXNode, IRNode: old.IRNode, PublicAddress: old.PublicAddress,
		Port: 9443, Target: old.Target, RouteListen: old.RouteListen, RouteID: old.RouteID,
		EXMetricsListen: old.EXMetricsListen, IRMetricsListen: old.IRMetricsListen, RecordShaping: old.RecordShaping}
	plan, err := s.BuildPlan(edit, now)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Replaces != old.ID {
		t.Fatalf("plan replaces %q, want %q", plan.Replaces, old.ID)
	}
	repl, _, err := s.CreateTunnelFromPlan(edit, plan.Hash, now)
	if err != nil {
		t.Fatal(err)
	}
	if repl.Port != 9443 || repl.Target != old.Target || repl.RouteID != old.RouteID || repl.RouteListen != old.RouteListen || repl.PublicAddress != old.PublicAddress {
		t.Fatalf("edit changed fields it should keep: old %+v new %+v", old, repl)
	}
}

func TestDashboardOffersEditForActiveTunnels(t *testing.T) {
	for _, want := range []string{`onclick="tnEdit(this.dataset.id)"`, "async function tnEdit(", "Object.assign({},tnEditBase,", "p.replaces"} {
		if !strings.Contains(dashboardHTML, want) {
			t.Errorf("dashboard lacks %s", want)
		}
	}
}
