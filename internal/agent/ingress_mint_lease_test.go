//go:build linux

package agent

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/release"
)

type mintFixture struct {
	agent *Agent
	priv  ed25519.PrivateKey
	job   agentjob.Job
	cmd   ingressauthority.CommandV1
	env   release.Envelope
	guard *ingressauthority.PendingGuard
	store *ingressauthority.ReservationStore
}

func newMintFixture(t *testing.T) *mintFixture {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	job := validSignedIngressV1(t)
	job.IssuedAt = job.IssuedAt.UTC().Truncate(time.Second)
	job.ExpiresAt = job.ExpiresAt.UTC().Truncate(time.Second)
	a, err := New(Config{BCCURL: "https://bcc.invalid", NodeID: job.NodeID, Token: "t", BCCJobKey: pub, StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	cmd, err := ingressV1Command(job)
	if err != nil {
		t.Fatal(err)
	}
	env, err := agentjob.Sign(priv, job)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "authority")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := ingressauthority.OpenReservationStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReservePending(cmd.Binding); err != nil {
		t.Fatal(err)
	}
	g, err := store.HoldPending(cmd.Binding)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = g.Close() })
	return &mintFixture{agent: a, priv: priv, job: job, cmd: cmd, env: env, guard: g, store: store}
}

// Fail-closed paths that need no installed policy: these must never mint.
func TestMintIngressLeaseFailsClosed(t *testing.T) {
	f := newMintFixture(t)
	ctx := context.Background()
	if l, err := f.agent.MintIngressLease(ctx, f.env, f.cmd, f.guard); err == nil || l != nil {
		t.Fatalf("minted without pinned policy/recorded intent: %v", err)
	}
	if l, err := f.agent.MintIngressLease(ctx, f.env, f.cmd, nil); err == nil || l != nil {
		t.Fatal("minted with nil guard")
	}
	if l, err := (*Agent)(nil).MintIngressLease(ctx, f.env, f.cmd, f.guard); err == nil || l != nil {
		t.Fatal("nil agent minted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if l, err := f.agent.MintIngressLease(cancelled, f.env, f.cmd, f.guard); err == nil || l != nil {
		t.Fatal("cancelled context minted")
	}
	// Signed by a non-enrolled key.
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	forged, err := agentjob.Sign(other, f.job)
	if err != nil {
		t.Fatal(err)
	}
	if l, err := f.agent.MintIngressLease(ctx, forged, f.cmd, f.guard); err == nil || l != nil {
		t.Fatal("job signed by unenrolled key minted")
	}
	// Wrong node: signed job for another node.
	wrongNode := f.job
	wrongNode.NodeID = "other-node"
	env, err := agentjob.Sign(f.priv, wrongNode)
	if err != nil {
		t.Fatal(err)
	}
	if l, err := f.agent.MintIngressLease(ctx, env, f.cmd, f.guard); err == nil || l != nil {
		t.Fatal("job for another node minted")
	}
	// Command whose binding node differs from this Agent.
	cmd := f.cmd
	cmd.Binding.NodeID = "other-node"
	if l, err := f.agent.MintIngressLease(ctx, f.env, cmd, f.guard); !errors.Is(err, ingressauthority.ErrBinding) || l != nil {
		t.Fatalf("binding for another node: %v", err)
	}
	// Generation drift between signed job and presented command.
	drift := f.job
	drift.Params = map[string]string{}
	for k, v := range f.job.Params {
		drift.Params[k] = v
	}
	drift.Params["generation"] = "2"
	driftCmd, derr := ingressV1Command(drift)
	if derr == nil {
		if l, err := f.agent.MintIngressLease(ctx, f.env, driftCmd, f.guard); err == nil || l != nil {
			t.Fatal("generation drift minted")
		}
	}
	// Expired.
	late := time.Now().Add(2 * time.Minute)
	f.agent.cfg.Now = func() time.Time { return late }
	if l, err := f.agent.MintIngressLease(ctx, f.env, f.cmd, f.guard); err == nil || l != nil {
		t.Fatal("expired signed job minted")
	}
	f.agent.cfg.Now = nil
	// Closed guard.
	if err := f.guard.Close(); err != nil {
		t.Fatal(err)
	}
	if l, err := f.agent.MintIngressLease(ctx, f.env, f.cmd, f.guard); err == nil || l != nil {
		t.Fatal("closed guard minted")
	}
}

// Positive path needs the fixed installed policy, so it only runs in the
// explicit disposable policy LAB (same opt-in as ingressauthority's LAB).
func TestMintIngressLeaseLABPositiveSingleUse(t *testing.T) {
	if os.Getenv("BAFT_ISOLATED_POLICY_LAB") != "1" {
		t.Skip("requires disposable Linux root with explicit LAB opt-in")
	}
	sentinel, err := os.ReadFile("/baft-policy-lab")
	if os.Geteuid() != 0 || err != nil || string(sentinel) != "isolated-policy-lab" {
		t.Fatal("refusing outside explicitly prepared disposable root")
	}
	f := newMintFixture(t)
	ctx := context.Background()
	if _, err := ingressauthority.ProvisionInstalledManagedNodePolicy(ctx, f.job.NodeID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll("/etc/baft") })
	if _, err := f.guard.BindInstalledPolicy(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.guard.RecordCommandIntent(ctx, f.cmd); err != nil {
		t.Fatal(err)
	}
	if err := f.guard.MarkApplying(ctx, f.cmd.Binding); err != nil {
		t.Fatal(err)
	}
	lease, err := f.agent.MintIngressLease(ctx, f.env, f.cmd, f.guard)
	if err != nil || lease == nil {
		t.Fatalf("enrolled signed held Apply refused: %v", err)
	}
	if err := lease.CheckLockedIngress(ctx, f.cmd); err != nil {
		t.Fatalf("minted lease not live: %v", err)
	}
	if l, err := f.agent.MintIngressLease(ctx, f.env, f.cmd, f.guard); !errors.Is(err, ingressauthority.ErrLeaseReplay) || l != nil {
		t.Fatalf("replay not refused: %v", err)
	}
	if err := f.guard.Close(); err != nil {
		t.Fatal(err)
	}
	if err := lease.CheckLockedIngress(ctx, f.cmd); err == nil {
		t.Fatal("lease outlived its guard")
	}
}
