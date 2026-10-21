package agent

import (
	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectPreparedIngressManifestNeverClaimsHostOwnership(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := ingressmanifest.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	spec := ingressmanifest.Spec{TunnelID: "tun-a", Generation: 1, Config: ingressmanifest.Config{Listen: "0.0.0.0:54619", Target: "127.0.0.1:1443"}}
	expected, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inspectPreparedIngressManifest(store, expected); err == nil {
		t.Fatal("absent manifest accepted")
	}
	written, err := store.Prepare(spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := inspectPreparedIngressManifest(store, written)
	if err != nil {
		t.Fatal(err)
	}
	if !verified.Prepared || !verified.BytesVerified || verified.LiveOwnershipVerified || verified.ReadyToApply {
		t.Fatalf("invalid host ownership assertion: %+v", verified)
	}
	wrong := written
	wrong.Generation++
	if _, err := inspectPreparedIngressManifest(store, wrong); err == nil {
		t.Fatal("stale proof accepted")
	}
}

func TestSignedIngressManifestInspectionRequiresIndependentProof(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := ingressmanifest.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	spec := ingressmanifest.Spec{TunnelID: "tun-a", Generation: 1, Config: ingressmanifest.Config{Listen: "0.0.0.0:54619", Target: "127.0.0.1:1443"}}
	proof, err := store.Prepare(spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	job := ingressPreflightJob()
	job.Action = agentjob.ActionPublicIngressInspect
	job.Params["tunnel_id"] = "tun-a"
	job.Params["public_port"] = "54619"
	job.Params["local_target"] = "127.0.0.1:1443"
	job.Params["generation"] = "1"
	job.Params["manifest_digest"] = proof.ManifestSHA256
	job.Params["listener_id"] = "listener-7"
	job.Params["firewall_id"] = "firewall-7"
	observed, err := inspectSignedIngressManifest(store, job, proof)
	if err != nil || !observed.Prepared || observed.LiveOwnershipVerified || observed.ReadyToApply {
		t.Fatalf("invalid read-only inspection: %+v %v", observed, err)
	}
	job.Params["public_port"] = "2053"
	if _, err := inspectSignedIngressManifest(store, job, proof); err == nil {
		t.Fatal("signed port disagrees with prepared manifest")
	}
	job.Params["public_port"] = "54619"
	job.Params["local_target"] = "127.0.0.1:7777"
	if _, err := inspectSignedIngressManifest(store, job, proof); err == nil {
		t.Fatal("signed target disagrees with prepared manifest")
	}
	job.Params["local_target"] = "127.0.0.1:1443"
	job.Params["manifest_digest"] = strings.Repeat("b", 64)
	if _, err := inspectSignedIngressManifest(store, job, proof); err == nil {
		t.Fatal("mismatched signed intent accepted")
	}
}
