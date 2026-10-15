package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zarkmakerburg/baft/internal/bcc"
)

func TestSignedCandidateIRFailurePreventsEitherServiceCutover(t *testing.T) {
	f := newFlow(t)
	old := f.run(f.create(f.plan()).ID)
	if old.Phase != bcc.TunnelActive {
		t.Fatal(old.Error)
	}
	before := map[string]string{}
	calls := map[string]int{}
	for _, n := range []*flowNode{f.ex, f.ir} {
		for _, path := range []string{filepath.Join(n.etc, "baft.yaml"), filepath.Join(n.etc, "baft.managed.json"), filepath.Join(n.tn.UnitDir, "baft.service"), filepath.Join(n.tn.StateDir, "tunnels", "generation")} {
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			before[path] = string(b)
		}
		calls[n.id] = len(n.sys.calls)
	}
	p := f.plan()
	p.PublicAddress = "localhost"
	tn := f.create(p)
	for _, n := range []*flowNode{f.ex, f.ir, f.ex} {
		if _, err := n.agent.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.tunnel(tn.ID); got.Phase != bcc.TunnelValidatingIR || len(got.Candidates) != 1 {
		t.Fatalf("separate staging: %+v", got)
	}
	path := filepath.Join(f.ir.tn.StateDir, "tunnels", tn.ID, "baft.new.yaml")
	if err := os.WriteFile(path, []byte("invalid: candidate\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.ir.agent.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	done := f.run(tn.ID)
	if done.Phase != bcc.TunnelRolledBack {
		t.Fatalf("failed preparation: %+v", done)
	}
	for path, want := range before {
		if b, err := os.ReadFile(path); err != nil || string(b) != want {
			t.Fatalf("preparation failure changed active artifact %s", path)
		}
	}
	for _, n := range []*flowNode{f.ex, f.ir} {
		for _, call := range n.sys.calls[calls[n.id]:] {
			if !strings.HasPrefix(call, "is-active ") && !strings.HasPrefix(call, "is-enabled ") && !strings.HasPrefix(call, "show ") {
				t.Fatalf("preparation operated active service: %s", call)
			}
		}
	}
	for _, j := range f.store.ListJobs() {
		if j.Params["tunnel_id"] == tn.ID && j.Type == bcc.JobTunnelCommitCandidate {
			t.Fatal("commit queued despite IR candidate failure")
		}
	}
	f.noSecretsInBCC()
}
