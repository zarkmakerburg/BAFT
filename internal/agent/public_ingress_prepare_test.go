package agent

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"github.com/zarkmakerburg/baft/internal/release"
)

func prepareJob(t *testing.T) (agentjob.Job, ingressmanifest.Proof) {
	t.Helper()
	now := time.Now().UTC()
	spec := ingressmanifest.Spec{TunnelID: "tun-a", Generation: 1, Config: ingressmanifest.Config{Listen: net.JoinHostPort("0.0.0.0", strconv.Itoa(54619)), Target: "127.0.0.1:1443", MaxClients: 128, DialTimeoutMS: 3000}}
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	j := agentjob.Job{SchemaVersion: agentjob.SchemaVersion, JobID: "prepare-a", NodeID: "ir-a", Action: agentjob.ActionPublicIngressPrepare, IssuedAt: now, ExpiresAt: now.Add(time.Minute), Params: map[string]string{
		"tunnel_id": "tun-a", "public_port": "54619", "local_target": "127.0.0.1:1443", "generation": "1",
		"manifest_digest": proof.ManifestSHA256, "listener_id": "listener-a", "firewall_id": "firewall-a",
	}}
	return j, proof
}

func TestSignedIngressPrepareWritesOnlyExpectedPrivateManifest(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	j, expected := prepareJob(t)
	pub, key, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	env, err := agentjob.Sign(key, j)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := (agentjob.Verifier{BCCKey: pub, NodeID: j.NodeID}).Verify(env, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	a := &Agent{cfg: Config{IngressManifestDir: dir}}
	detail, output, err := a.executeFull(context.Background(), verified)
	if err != nil || !strings.Contains(detail, "listener and firewall unchanged") {
		t.Fatalf("prepare: %s %v", detail, err)
	}
	var receipt publicIngressPrepareReceipt
	if err := json.Unmarshal([]byte(output), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Proof != expected || receipt.JobID != j.JobID || !receipt.PreparedBytesMatch || receipt.LiveOwnershipVerified || receipt.ReadyToApply {
		t.Fatalf("unsafe receipt: %+v", receipt)
	}
	store, err := ingressmanifest.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	inspection, err := store.Inspect(expected)
	if err != nil || !inspection.PreparedBytesMatch || inspection.LiveOwnershipVerified || inspection.ReadyToApply {
		t.Fatalf("stored manifest: %+v %v", inspection, err)
	}
	if _, _, err := a.executeFull(context.Background(), verified); err != nil {
		t.Fatalf("identical retry failed: %v", err)
	}
}

func TestIngressPrepareFailsClosedBeforeFileWrite(t *testing.T) {
	j, _ := prepareJob(t)
	if _, err := (&Agent{}).publicIngressPrepare(context.Background(), j); err == nil {
		t.Fatal("unconfigured manifest store accepted prepare")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	a := &Agent{cfg: Config{IngressManifestDir: dir}}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.publicIngressPrepare(cancelled, j); err == nil {
		t.Fatal("cancelled prepare accepted")
	}
	j.Params["manifest_digest"] = strings.Repeat("b", 64)
	if _, err := a.publicIngressPrepare(context.Background(), j); err == nil {
		t.Fatal("wrong signed digest accepted")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("wrong digest wrote files: %+v %v", entries, err)
	}
	j, _ = prepareJob(t)
	j.Params["generation"] = "2"
	if _, err := a.publicIngressPrepare(context.Background(), j); err == nil {
		t.Fatal("generation advance without previous proof accepted")
	}
	j, _ = prepareJob(t)
	j.Params["instance_id"] = "other-instance"
	if _, err := a.publicIngressPrepare(context.Background(), j); err == nil {
		t.Fatal("manifest digest reused for another instance")
	}
}
