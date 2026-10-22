package agent

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"github.com/zarkmakerburg/baft/internal/release"
)

func TestSignedIngressCancelFencesPreparedOrUnseenGeneration(t *testing.T) {
	for _, prepared := range []bool{false, true} {
		t.Run(map[bool]string{false: "unseen", true: "prepared"}[prepared], func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Chmod(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			j, expected := prepareJob(t)
			store, err := ingressmanifest.Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			spec := ingressmanifest.Spec{TunnelID: "tun-a", Generation: 1, Config: ingressmanifest.Config{Listen: "0.0.0.0:54619", Target: "127.0.0.1:1443", MaxClients: 128, DialTimeoutMS: 3000}}
			if prepared {
				if proof, err := store.Prepare(spec, nil); err != nil || proof != expected {
					t.Fatalf("prepare: %+v %v", proof, err)
				}
			}
			j.Action, j.JobID = agentjob.ActionPublicIngressCancel, "cancel-a"
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
			_, output, err := a.executeFull(context.Background(), verified)
			if err != nil {
				t.Fatal(err)
			}
			var receipt publicIngressCancelReceipt
			if err := json.Unmarshal([]byte(output), &receipt); err != nil {
				t.Fatal(err)
			}
			want, err := ingressmanifest.PreviewCancellation(spec)
			if err != nil {
				t.Fatal(err)
			}
			if receipt.JobID != j.JobID || receipt.NodeID != j.NodeID || receipt.Proof != want || receipt.LiveOwnershipVerified || receipt.ReadyToApply {
				t.Fatalf("unsafe cancellation receipt: %+v", receipt)
			}
			inspection, err := store.Inspect(want)
			if err != nil || inspection.Manifest.Phase != ingressmanifest.PhaseCancelled || inspection.PreparedBytesMatch {
				t.Fatalf("tombstone: %+v %v", inspection, err)
			}
			if _, err := store.Prepare(spec, nil); err == nil {
				t.Fatal("cancelled generation resurrected")
			}
			if _, _, err := a.executeFull(context.Background(), verified); err != nil {
				t.Fatalf("identical cancellation retry failed: %v", err)
			}
		})
	}
}

func TestIngressCancelRejectsWrongProofBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	j, _ := prepareJob(t)
	j.Action = agentjob.ActionPublicIngressCancel
	j.Params["manifest_digest"] = strings.Repeat("b", 64)
	if _, err := (&Agent{cfg: Config{IngressManifestDir: dir}}).publicIngressCancel(context.Background(), j); err == nil {
		t.Fatal("wrong signed digest accepted")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("wrong digest wrote files: %+v %v", entries, err)
	}
}

// A cancellation received before the corresponding prepare must permanently
// fence that signed generation, even when the delayed prepare is replayed.
func TestIngressCancelBeforeDelayedSignedPrepareCannotResurrect(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	prepare, expected := prepareJob(t)
	cancel := prepare
	cancel.JobID = "cancel-before-prepare"
	cancel.Action = agentjob.ActionPublicIngressCancel
	pub, key, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	a := &Agent{cfg: Config{IngressManifestDir: dir}}
	for _, j := range []agentjob.Job{cancel, prepare, cancel} {
		env, err := agentjob.Sign(key, j)
		if err != nil {
			t.Fatal(err)
		}
		verified, err := (agentjob.Verifier{BCCKey: pub, NodeID: j.NodeID}).Verify(env, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = a.executeFull(context.Background(), verified)
		if j.Action == agentjob.ActionPublicIngressPrepare {
			if err == nil {
				t.Fatal("delayed signed prepare resurrected cancelled generation")
			}
		} else if err != nil {
			t.Fatalf("cancellation failed: %v", err)
		}
	}
	store, err := ingressmanifest.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	proof, err := ingressmanifest.PreviewCancellation(ingressmanifest.Spec{TunnelID: "tun-a", Generation: 1, Config: ingressmanifest.Config{Listen: "0.0.0.0:54619", Target: "127.0.0.1:1443", MaxClients: 128, DialTimeoutMS: 3000}})
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := store.Inspect(proof)
	if err != nil || inspection.PreparedBytesMatch || inspection.Manifest.Phase != ingressmanifest.PhaseCancelled {
		t.Fatalf("fence not durable: %+v %v (original %+v)", inspection, err, expected)
	}
}
