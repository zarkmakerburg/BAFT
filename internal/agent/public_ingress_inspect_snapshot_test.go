package agent

import (
	"encoding/json"
	"github.com/zarkmakerburg/baft/internal/agentjob"
	"strings"
	"testing"
)

func TestPublicIngressInspectSnapshotBindsSignedIntentWithoutReadiness(t *testing.T) {
	j := ingressPreflightJob()
	j.Action = agentjob.ActionPublicIngressInspect
	j.Params["generation"] = "7"
	j.Params["manifest_digest"] = strings.Repeat("a", 64)
	j.Params["listener_id"] = "baft-listener-7"
	j.Params["firewall_id"] = "baft-firewall-7"
	ev := publicIngressPreflightEvidence{SchemaVersion: 1, Scope: "agent_network_namespace_snapshot", PublicPortState: "occupied"}
	raw, err := marshalPublicIngressInspectSnapshot(j, ev)
	if err != nil {
		t.Fatal(err)
	}
	var result publicIngressInspectSnapshot
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	if result.JobID != j.JobID || result.NodeID != j.NodeID || result.Generation != "7" || result.ManifestDigest != j.Params["manifest_digest"] || result.ListenerID != j.Params["listener_id"] || result.FirewallID != j.Params["firewall_id"] {
		t.Fatalf("incorrect signed intent correlation: %+v", result)
	}
	if result.LiveOwnershipVerified || result.ReadyToApply || result.PublicIngressPreflight.OwnershipVerified {
		t.Fatal("unsupported ownership claim")
	}
	j.Params["generation"] = "0"
	if _, err := marshalPublicIngressInspectSnapshot(j, ev); err == nil {
		t.Fatal("invalid job accepted")
	}
}
