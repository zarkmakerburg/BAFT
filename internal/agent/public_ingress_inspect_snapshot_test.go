package agent

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"os"
	"path/filepath"
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

func TestPublicIngressInspectHandlerFailsClosedWithoutTrustedStore(t *testing.T) {
	j := ingressPreflightJob()
	j.Action = agentjob.ActionPublicIngressInspect
	j.Params["generation"] = "1"
	j.Params["manifest_digest"] = strings.Repeat("a", 64)
	j.Params["listener_id"] = "listener-1"
	j.Params["firewall_id"] = "firewall-1"
	a := &Agent{}
	result, err := a.publicIngressPreflight(context.Background(), j)
	if err == nil || result != "" {
		t.Fatalf("unverified inspection succeeded: %q %v", result, err)
	}
}

func TestIngressInspectHandlerChecksTrustedManifestButNeverClaimsLiveOwner(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := ingressmanifest.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	spec := ingressmanifest.Spec{TunnelID: "tun-a", Generation: 1, Config: ingressmanifest.Config{Listen: "0.0.0.0:54619", Target: "127.0.0.1:1443"}}
	proof, err := store.Prepare(spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	j := ingressPreflightJob()
	j.Action = agentjob.ActionPublicIngressInspect
	j.Params["tunnel_id"] = "tun-a"
	j.Params["public_port"] = "54619"
	j.Params["local_target"] = "127.0.0.1:1443"
	j.Params["generation"] = "1"
	j.Params["manifest_digest"] = proof.ManifestSHA256
	j.Params["listener_id"] = "listener-1"
	j.Params["firewall_id"] = "firewall-1"
	a := &Agent{cfg: Config{IngressManifestDir: dir, IngressExpectedProof: func(string) (ingressmanifest.Proof, error) { return proof, nil }}}
	output, err := a.publicIngressPreflight(context.Background(), j)
	if err == nil || output != "" || !strings.Contains(err.Error(), "live listener") {
		t.Fatalf("prepared bytes incorrectly accepted as host proof: %q %v", output, err)
	}
	j.Params["manifest_digest"] = strings.Repeat("b", 64)
	output, err = a.publicIngressPreflight(context.Background(), j)
	if err == nil || output != "" || strings.Contains(err.Error(), "live listener") {
		t.Fatalf("mismatched proof did not fail early: %q %v", output, err)
	}
}

func TestIngressInspectCancellationDuringProofResolutionDoesNotOpenStore(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	j := ingressPreflightJob()
	j.Action = agentjob.ActionPublicIngressInspect
	j.Params["generation"] = "1"
	j.Params["manifest_digest"] = strings.Repeat("a", 64)
	j.Params["listener_id"] = "listener-1"
	j.Params["firewall_id"] = "firewall-1"
	// A nonexistent directory must not be opened once the trusted resolver
	// cancels the request. The cancellation is the authoritative failure.
	a := &Agent{cfg: Config{IngressManifestDir: filepath.Join(t.TempDir(), "missing"), IngressExpectedProof: func(string) (ingressmanifest.Proof, error) {
		cancel()
		return ingressmanifest.Proof{}, nil
	}}}
	output, err := a.publicIngressPreflight(ctx, j)
	if !errors.Is(err, context.Canceled) || output != "" {
		t.Fatalf("cancelled inspection continued to store: output=%q err=%v", output, err)
	}
}
