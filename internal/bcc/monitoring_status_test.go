package bcc

import (
	"path/filepath"
	"testing"
	"time"
)

// A node that answers the TCP health probe, or whose agent polls, is online
// even before telemetry arrives; only a node with no evidence is unknown.
func TestMonitoringStatusFollowsHealthProbeWithoutTelemetry(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"n-up", "n-down", "n-new", "n-agent"} {
		if _, err := store.UpsertNode(Node{ID: id, Address: "127.0.0.1:1", Role: "foreign"}, ""); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	if err := store.SetHealth("n-up", "up", 12, now); err != nil {
		t.Fatal(err)
	}
	if err := store.SetHealth("n-down", "down", -1, now); err != nil {
		t.Fatal(err)
	}
	if err := store.SetHealth("n-agent", "down", -1, now); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	n := store.st.Nodes["n-agent"]
	n.AgentSeen = now.Add(-30 * time.Second)
	store.st.Nodes["n-agent"] = n
	store.mu.Unlock()
	got := map[string]string{}
	for _, m := range store.MonitoringSnapshot(now, 3*time.Minute) {
		got[m.NodeID] = m.Status
	}
	want := map[string]string{"n-up": "up", "n-down": "down", "n-new": "unknown", "n-agent": "up"}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s status=%q, want %q", id, got[id], w)
		}
	}
}
