package bcc

import (
	"encoding/json"
	"path/filepath"
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

func TestInspectAckRejectsInvalidEvidenceWithoutCompletingJob(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bcc.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertNode(Node{ID: "ir-node", Address: "127.0.0.1:22000", Role: "foreign"}, "tok-ir"); err != nil {
		t.Fatal(err)
	}
	params := map[string]string{"tunnel_id": "tun-test", "public_port": "2053", "local_target": "127.0.0.1:1443", "generation": "7", "manifest_digest": strings.Repeat("a", 64), "listener_id": "baft-listener-7", "firewall_id": "baft-firewall-7"}
	s.mu.Lock()
	job := s.newJobLocked(Job{Type: "public_ingress_inspect", NodeID: "ir-node", Params: params})
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()
	bad := `{"job_id":"not-this-job"}`
	if err := s.AckJobOutput("ir-node", "tok-ir", job.ID, "succeeded", "done", bad); err == nil {
		t.Fatal("invalid ACK accepted")
	}
	got := s.st.Jobs[job.ID]
	if got.Status != "queued" || got.Output != "" {
		t.Fatalf("invalid ACK mutated job: %+v", got)
	}
	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	persisted := reopened.st.Jobs[job.ID]
	if persisted.Status != "queued" || persisted.Output != "" {
		t.Fatalf("invalid ACK persisted: %+v", persisted)
	}
}

func TestInspectAckValidReadOnlySnapshotPersistsWithoutReadiness(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bcc.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertNode(Node{ID: "ir-node", Address: "127.0.0.1:22000", Role: "foreign"}, "tok-ir"); err != nil {
		t.Fatal(err)
	}
	params := map[string]string{"tunnel_id": "tun-test", "public_port": "2053", "local_target": "127.0.0.1:1443", "generation": "7", "manifest_digest": strings.Repeat("a", 64), "listener_id": "baft-listener-7", "firewall_id": "baft-firewall-7"}
	s.mu.Lock()
	job := s.newJobLocked(Job{Type: "public_ingress_inspect", NodeID: "ir-node", Params: params})
	err = s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	preflight := map[string]any{"schema_version": 1, "tunnel_id": "tun-test", "public_port": 2053, "local_target": "127.0.0.1:1443", "scope": "agent_network_namespace_snapshot", "socket_inventory_complete": true, "public_port_state": "not_observed", "public_listener_count": 0, "target_listener_observed": false, "ownership_verified": false, "firewall_tools": []string{}, "firewall_policy_verified": false, "ready_to_apply": false, "limitations": []string{"Ownership unverified"}}
	envelope := map[string]any{"preflight": preflight, "job_id": job.ID, "node_id": "ir-node", "generation": "7", "manifest_digest": params["manifest_digest"], "listener_id": params["listener_id"], "firewall_id": params["firewall_id"], "live_ownership_verified": false, "ready_to_apply": false}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AckJobOutput("ir-node", "tok-ir", job.ID, "succeeded", "read-only inspect", string(raw)); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	persisted := reopened.st.Jobs[job.ID]
	if persisted.Status != "succeeded" || persisted.Output != string(raw) {
		t.Fatalf("receipt not persisted: %+v", persisted)
	}
	if err := reopened.AckJobOutput("ir-node", "tok-ir", job.ID, "succeeded", "replay", string(raw)); err == nil {
		t.Fatal("duplicate ACK accepted")
	}
	if err := validatePublicIngressInspectSnapshot(persisted.Output, job.ID, "ir-node", params); err != nil {
		t.Fatal(err)
	}
}
