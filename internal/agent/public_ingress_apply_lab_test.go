package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"github.com/zarkmakerburg/baft/internal/release"
)

type fakeIngressLAB struct {
	apply, retire int
	err           error
	onApply       func()
}

func (f *fakeIngressLAB) Apply(_ context.Context, _ ingressmanifest.Spec, _ ingressmanifest.Proof) error {
	f.apply++
	if f.onApply != nil {
		f.onApply()
	}
	return f.err
}
func (f *fakeIngressLAB) Retire(_ context.Context, _ ingressmanifest.Spec, _ ingressmanifest.Proof) error {
	f.retire++
	return f.err
}

func TestPublicIngressLABProofAndDisabledGate(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	j, proof := prepareJob(t)
	j.Action, j.JobID = agentjob.ActionPublicIngressApply, "apply-a"
	pub, key, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	env, err := agentjob.Sign(key, j)
	if err != nil {
		t.Fatal(err)
	}
	j, err = (agentjob.Verifier{BCCKey: pub, NodeID: j.NodeID}).Verify(env, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeIngressLAB{}
	a := &Agent{cfg: Config{NodeID: j.NodeID, IngressManifestDir: dir, IngressLABExecutor: fake}}
	if _, _, err := (&Agent{cfg: Config{NodeID: j.NodeID, IngressManifestDir: dir}}).executeFull(context.Background(), j); err == nil {
		t.Fatal("default agent accepted live mutation")
	}
	if _, _, err := a.executeFull(context.Background(), j); err == nil {
		t.Fatal("missing prepared proof accepted")
	}
	store, err := ingressmanifest.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	spec := ingressmanifest.Spec{TunnelID: "tun-a", Generation: 1, Config: ingressmanifest.Config{Listen: "0.0.0.0:54619", Target: "127.0.0.1:1443", MaxClients: 128, DialTimeoutMS: 3000}}
	if got, err := store.Prepare(spec, nil); err != nil || got != proof {
		t.Fatalf("prepare: %+v %v", got, err)
	}
	store.Close()
	bad := j
	bad.Params = map[string]string{}
	for k, v := range j.Params {
		bad.Params[k] = v
	}
	bad.Params["manifest_digest"] = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, _, err := a.executeFull(context.Background(), bad); err == nil {
		t.Fatal("wrong digest accepted")
	}
	wrongNode := j
	wrongNode.NodeID = "other"
	if _, _, err := a.executeFull(context.Background(), wrongNode); err == nil {
		t.Fatal("wrong node accepted")
	}
	if fake.apply != 0 {
		t.Fatal("invalid jobs invoked executor")
	}
	_, output, err := a.executeFull(context.Background(), j)
	if err != nil {
		t.Fatal(err)
	}
	var receipt publicIngressLABReceipt
	if err := json.Unmarshal([]byte(output), &receipt); err != nil {
		t.Fatal(err)
	}
	if !receipt.LiveOwnershipVerified || receipt.Proof != proof || fake.apply != 1 {
		t.Fatalf("receipt or invocation: %+v %+v", receipt, fake)
	}
	fake.err = errors.New("kernel verification failed")
	if output, err := a.publicIngressLAB(context.Background(), j); err == nil || output != "" {
		t.Fatal("failed apply claimed success")
	}
	j.Action = agentjob.ActionPublicIngressRetire
	fake.err = nil
	_, output, err = a.executeFull(context.Background(), j)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(output), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.LiveOwnershipVerified || fake.retire != 1 {
		t.Fatalf("unsafe retire receipt: %+v %+v", receipt, fake)
	}
}

func TestPublicIngressLABCancelledGenerationCannotApply(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	j, proof := prepareJob(t)
	store, err := ingressmanifest.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	spec := ingressmanifest.Spec{TunnelID: "tun-a", Generation: 1, Config: ingressmanifest.Config{Listen: "0.0.0.0:54619", Target: "127.0.0.1:1443", MaxClients: 128, DialTimeoutMS: 3000}}
	if _, err := store.Prepare(spec, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Cancel(proof); err != nil {
		t.Fatal(err)
	}
	store.Close()
	j.Action = agentjob.ActionPublicIngressApply
	fake := &fakeIngressLAB{}
	a := &Agent{cfg: Config{NodeID: j.NodeID, IngressManifestDir: dir, IngressLABExecutor: fake}}
	if _, err := a.publicIngressLAB(context.Background(), j); err == nil || fake.apply != 0 {
		t.Fatal("cancelled generation resurrected")
	}
}

func TestPublicIngressLABCancelledGenerationStillAllowsExactRetire(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	j, proof := prepareJob(t)
	fake := &fakeIngressLAB{}
	a := &Agent{cfg: Config{NodeID: j.NodeID, IngressManifestDir: dir, IngressLABExecutor: fake}}
	if _, err := a.publicIngressPrepare(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	j.Action = agentjob.ActionPublicIngressApply
	if _, err := a.publicIngressLAB(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	j.Action = agentjob.ActionPublicIngressCancel
	if _, err := a.publicIngressCancel(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	j.Action = agentjob.ActionPublicIngressRetire
	fake.err = errors.New("live owner cannot be verified")
	if output, err := a.publicIngressLAB(context.Background(), j); err == nil || output != "" {
		t.Fatalf("tombstone bypassed executor ownership: %q %v", output, err)
	}
	fake.err = nil
	output, err := a.publicIngressLAB(context.Background(), j)
	if err != nil {
		t.Fatalf("tombstone prevented exact-owner cleanup: %v", err)
	}
	var receipt publicIngressLABReceipt
	if err := json.Unmarshal([]byte(output), &receipt); err != nil || receipt.Proof != proof || receipt.LiveOwnershipVerified || fake.retire != 2 {
		t.Fatalf("unsafe retire receipt: %s %v", output, err)
	}
	j.Action = agentjob.ActionPublicIngressApply
	if _, err := a.publicIngressLAB(context.Background(), j); err == nil || fake.apply != 1 {
		t.Fatal("retirement resurrected cancelled generation")
	}
}

func TestPublicIngressLABRetireRejectsForeignOrCorruptTombstone(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		t.Run(map[bool]string{false: "foreign", true: "corrupt"}[corrupt], func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Chmod(dir, 0700); err != nil {
				t.Fatal(err)
			}
			j, _ := prepareJob(t)
			store, err := ingressmanifest.Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			// Same owner and generation, but a different target: this is a valid
			// canonical tombstone and cannot authorize the original signed job.
			foreign := ingressmanifest.Spec{TunnelID: "tun-a", Generation: 1, Config: ingressmanifest.Config{Listen: "0.0.0.0:54619", Target: "127.0.0.1:2443", MaxClients: 128, DialTimeoutMS: 3000}}
			if _, err := store.FenceUnseen(foreign); err != nil {
				t.Fatal(err)
			}
			_ = store.Close()
			if corrupt {
				files, err := filepath.Glob(filepath.Join(dir, "*.json"))
				if err != nil || len(files) != 1 {
					t.Fatalf("manifest files: %v %v", files, err)
				}
				if err := os.WriteFile(files[0], []byte("{"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			j.Action = agentjob.ActionPublicIngressRetire
			fake := &fakeIngressLAB{}
			a := &Agent{cfg: Config{NodeID: j.NodeID, IngressManifestDir: dir, IngressLABExecutor: fake}}
			if output, err := a.publicIngressLAB(context.Background(), j); err == nil || output != "" || fake.apply != 0 || fake.retire != 0 {
				t.Fatalf("invalid tombstone reached executor: %q %v %+v", output, err, fake)
			}
		})
	}
}

func TestPublicIngressLABCancelDuringApplyRollsBack(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	j, proof := prepareJob(t)
	store, err := ingressmanifest.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	spec := ingressmanifest.Spec{TunnelID: "tun-a", Generation: 1, Config: ingressmanifest.Config{Listen: "0.0.0.0:54619", Target: "127.0.0.1:1443", MaxClients: 128, DialTimeoutMS: 3000}}
	if _, err := store.Prepare(spec, nil); err != nil {
		t.Fatal(err)
	}
	store.Close()
	j.Action = agentjob.ActionPublicIngressApply
	fake := &fakeIngressLAB{onApply: func() {
		other, err := ingressmanifest.Open(dir)
		if err != nil {
			t.Error(err)
			return
		}
		defer other.Close()
		if _, err := other.Cancel(proof); err != nil {
			t.Error(err)
		}
	}}
	a := &Agent{cfg: Config{NodeID: j.NodeID, IngressManifestDir: dir, IngressLABExecutor: fake}}
	if output, err := a.publicIngressLAB(context.Background(), j); err == nil || output != "" || fake.apply != 1 || fake.retire != 1 {
		t.Fatalf("cancelled apply escaped rollback: %q %v %+v", output, err, fake)
	}
}

func TestPublicIngressLABCancelContextDuringApplyRollsBack(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	j, proof := prepareJob(t)
	store, err := ingressmanifest.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	spec := ingressmanifest.Spec{TunnelID: "tun-a", Generation: 1, Config: ingressmanifest.Config{Listen: "0.0.0.0:54619", Target: "127.0.0.1:1443", MaxClients: 128, DialTimeoutMS: 3000}}
	if _, err := store.Prepare(spec, nil); err != nil {
		t.Fatal(err)
	}
	store.Close()
	j.Action = agentjob.ActionPublicIngressApply
	ctx, cancel := context.WithCancel(context.Background())
	fake := &fakeIngressLAB{onApply: cancel}
	a := &Agent{cfg: Config{NodeID: j.NodeID, IngressManifestDir: dir, IngressLABExecutor: fake}}
	output, err := a.publicIngressLAB(ctx, j)
	if err == nil || output != "" || fake.apply != 1 || fake.retire != 1 {
		t.Fatalf("cancelled Apply returned live ownership: output=%q err=%v apply=%d retire=%d proof=%+v", output, err, fake.apply, fake.retire, proof)
	}
}
