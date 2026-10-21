package bcc

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPublicIngressInspectSnapshotRejectsUnverifiedAndMismatchedIntent(t *testing.T) {
	params := map[string]string{"tunnel_id": "tun-test", "public_port": "2053", "local_target": "127.0.0.1:1443", "generation": "7", "manifest_digest": strings.Repeat("a", 64), "listener_id": "baft-listener-7", "firewall_id": "baft-firewall-7"}
	preflight := map[string]any{"schema_version": 1, "tunnel_id": "tun-test", "public_port": 2053, "local_target": "127.0.0.1:1443", "scope": "agent_network_namespace_snapshot", "socket_inventory_complete": true, "public_port_state": "not_observed", "public_listener_count": 0, "target_listener_observed": false, "ownership_verified": false, "firewall_tools": []string{}, "firewall_policy_verified": false, "ready_to_apply": false, "limitations": []string{"Cannot prove live ownership."}}
	envelope := map[string]any{"preflight": preflight, "job_id": "job-a", "node_id": "ir-node", "generation": "7", "manifest_digest": strings.Repeat("a", 64), "listener_id": "baft-listener-7", "firewall_id": "baft-firewall-7", "live_ownership_verified": false, "ready_to_apply": false}
	check := func(e map[string]any) error {
		b, _ := json.Marshal(e)
		return validatePublicIngressInspectSnapshot(string(b), "job-a", "ir-node", params)
	}
	if err := check(envelope); err != nil {
		t.Fatalf("valid non-authoritative receipt rejected: %v", err)
	}
	for _, key := range []string{"job_id", "node_id", "generation", "manifest_digest", "listener_id", "firewall_id"} {
		t.Run(key, func(t *testing.T) {
			e := map[string]any{}
			for k, v := range envelope {
				e[k] = v
			}
			e[key] = "changed"
			if err := check(e); err == nil {
				t.Fatal("mismatched intent accepted")
			}
		})
	}
	for _, key := range []string{"live_ownership_verified", "ready_to_apply"} {
		t.Run(key, func(t *testing.T) {
			e := map[string]any{}
			for k, v := range envelope {
				e[k] = v
			}
			e[key] = true
			if err := check(e); err == nil {
				t.Fatal("unsupported readiness accepted")
			}
		})
	}
}

func TestInspectSnapshotRejectsDuplicateAndTrailingJSON(t *testing.T) {
	params := map[string]string{"tunnel_id": "tun-a", "public_port": "54619", "local_target": "127.0.0.1:1443", "generation": "7", "manifest_digest": strings.Repeat("a", 64), "listener_id": "unit-a", "firewall_id": "rule-a"}
	for _, output := range []string{`{"job_id":"job-a","job_id":"job-a"}`, `{"job_id":"job-a"} {}`} {
		if err := validatePublicIngressInspectSnapshot(output, "job-a", "ir-a", params); err == nil {
			t.Fatal("ambiguous JSON accepted")
		}
	}
}
