package bcc

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInspectSnapshotCannotClaimOwnership(t *testing.T) {
	params := map[string]string{"tunnel_id": "tun-a", "public_port": "54619", "local_target": "127.0.0.1:1443", "generation": "7", "manifest_digest": strings.Repeat("a", 64), "listener_id": "unit-a", "firewall_id": "rule-a"}
	preflight := map[string]any{"schema_version": 1, "tunnel_id": "tun-a", "public_port": 54619, "local_target": "127.0.0.1:1443", "scope": "agent_network_namespace_snapshot", "socket_inventory_complete": true, "public_port_state": "not_observed", "public_listener_count": 0, "target_listener_observed": false, "ownership_verified": false, "firewall_tools": []string{}, "firewall_policy_verified": false, "ready_to_apply": false, "limitations": []string{"Unverified"}}
	receipt := map[string]any{"preflight": preflight, "job_id": "job-a", "node_id": "ir-a", "generation": "7", "manifest_digest": params["manifest_digest"], "listener_id": "unit-a", "firewall_id": "rule-a", "live_ownership_verified": false, "ready_to_apply": false}
	check := func(want bool) {
		t.Helper()
		b, _ := json.Marshal(receipt)
		err := validatePublicIngressInspectSnapshot(string(b), "job-a", "ir-a", params)
		if (err == nil) != want {
			t.Fatalf("accepted=%v want=%v err=%v", err == nil, want, err)
		}
	}
	check(true)
	receipt["live_ownership_verified"] = true
	check(false)
	receipt["live_ownership_verified"] = false
	receipt["ready_to_apply"] = true
	check(false)
	receipt["ready_to_apply"] = false
	receipt["generation"] = "8"
	check(false)
}
