package bcc

import (
	"strings"
	"testing"
	"time"
)

// A live legacy Arno→Germany pair must not be retired, restarted, or
// superseded when a separate scoped Arno→second-EX tunnel is removed.
func TestScopedSiblingDecommissionPreservesActiveLegacy(t *testing.T) {
	s := multiInstanceStore(t)
	now := time.Now().UTC()
	legacy := activeLegacyForCoexistence(t, s, now)
	scoped, err := s.CreateTunnel(testCoexistenceRequest(), now)
	if err != nil {
		t.Fatal(err)
	}

	s.mu.Lock()
	for _, id := range []string{"ir-1", "ex-2"} {
		n := s.st.Nodes[id]
		n.AgentSeen = now
		s.st.Nodes[id] = n
	}
	scoped.Phase = TunnelActive
	scoped.ObservedGen = map[string]int{"ir-1": 4, "ex-2": 6}
	scoped.Digests = map[string]NodeDigests{
		"ir-1": {Config: strings.Repeat("a", 64), Unit: strings.Repeat("b", 64), Marker: strings.Repeat("c", 64)},
		"ex-2": {Config: strings.Repeat("d", 64), Unit: strings.Repeat("e", 64), Marker: strings.Repeat("f", 64)},
	}
	s.st.Tunnels[scoped.ID] = scoped
	s.mu.Unlock()

	plan, err := s.BuildDecommissionPlan(scoped.ID, now)
	if err != nil || !plan.OK || plan.InstanceID != scoped.InstanceID {
		t.Fatalf("scoped decommission plan %+v err=%v", plan, err)
	}
	if len(plan.Nodes) != 2 || plan.Nodes[0].NodeID != "ir-1" || plan.Nodes[1].NodeID != "ex-2" {
		t.Fatalf("wrong retirement order/targets: %+v", plan.Nodes)
	}
	started, _, err := s.StartDecommissionFromPlan(scoped.ID, plan.Hash, now)
	if err != nil || started.Phase != TunnelDecommissioningIR {
		t.Fatalf("start scoped retirement phase=%s err=%v", started.Phase, err)
	}

	irJobs := pullAll(t, s, "ir-1")
	if len(irJobs) != 1 || irJobs[0].Type != JobTunnelRetire ||
		irJobs[0].Params["instance_id"] != scoped.InstanceID ||
		irJobs[0].Params["tunnel_id"] != scoped.ID {
		t.Fatalf("IR retirement targeted wrong instance: %+v", irJobs)
	}
	if err = s.AckJobOutput("ir-1", "tok-ir-1", irJobs[0].ID, "succeeded", retireMessage(scoped, "ir-1"), ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AdvanceTunnels(now); err != nil {
		t.Fatal(err)
	}
	// The synthetic ACTIVE state may retain an initial prepare job from
	// CreateTunnel; inspect the decommission job, not unrelated setup work.
	exJobs := []Job{}
	for _, j := range pullAll(t, s, "ex-2") {
		if j.Type == JobTunnelRetire {
			exJobs = append(exJobs, j)
		}
	}
	if len(exJobs) != 1 || exJobs[0].Type != JobTunnelRetire ||
		exJobs[0].Params["instance_id"] != scoped.InstanceID ||
		exJobs[0].Params["tunnel_id"] != scoped.ID {
		t.Fatalf("EX retirement targeted wrong instance: %+v", exJobs)
	}
	if err = s.AckJobOutput("ex-2", "tok-ex-2", exJobs[0].ID, "succeeded", retireMessage(scoped, "ex-2"), ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AdvanceTunnels(now); err != nil {
		t.Fatal(err)
	}

	done, _ := s.GetTunnel(scoped.ID)
	old, _ := s.GetTunnel(legacy.ID)
	if done.Phase != TunnelDecommissioned || old.Phase != TunnelActive {
		t.Fatalf("sibling retirement changed legacy: scoped=%s legacy=%s", done.Phase, old.Phase)
	}
	for _, j := range s.ListJobs() {
		if j.Type == JobTunnelRetire && j.Params["tunnel_id"] == legacy.ID {
			t.Fatalf("legacy tunnel must never receive sibling retire job: %+v", j)
		}
	}
}
