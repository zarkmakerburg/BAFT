//go:build linux

package ingressauthority

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Positive lease lifecycle needs the fixed installed policy path, so it only
// runs in the explicit disposable policy LAB (skipped elsewhere).
func TestMintLeaseLABSingleUseRevokedOnCloseAndUncertainty(t *testing.T) {
	isolatedPolicyBootstrapLAB(t)
	ctx := context.Background()
	if _, err := ProvisionInstalledManagedNodePolicy(ctx, "ir-1"); err != nil {
		t.Fatal(err)
	}
	b := bindingFixture(t)
	store, err := OpenReservationStore(privateReservationDir(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReservePending(b); err != nil {
		t.Fatal(err)
	}
	g, err := store.HoldPending(b)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.BindInstalledPolicy(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	apply := CommandV1{IngressProtocol: ProtocolV1, JobID: "apply-mint-1", Action: "public_ingress_apply",
		IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Binding: b}
	if err := g.RecordCommandIntent(ctx, apply); err != nil {
		t.Fatal(err)
	}
	ev := signedEvidence(t, apply)
	// Apply needs durable APPLYING_UNVERIFIED first.
	if _, err := g.MintLease(ctx, ev, apply); err == nil {
		t.Fatal("Apply lease minted before MarkApplying")
	}
	if err := g.MarkApplying(ctx, b); err != nil {
		t.Fatal(err)
	}
	lease, err := g.MintLease(ctx, ev, apply)
	if err != nil || lease == nil {
		t.Fatalf("held, signed, pinned Apply refused: %v", err)
	}
	if err := lease.CheckLockedIngress(ctx, apply); err != nil {
		t.Fatalf("live lease denied: %v", err)
	}
	if _, err := g.MintLease(ctx, ev, apply); !errors.Is(err, ErrLeaseReplay) {
		t.Fatalf("second mint for same signed job not a replay: %v", err)
	}
	drift := apply
	drift.Binding.PlanSHA256 = "b" + apply.Binding.PlanSHA256[1:]
	if err := lease.CheckLockedIngress(ctx, drift); err == nil {
		t.Fatal("lease accepted drifted binding")
	}
	wrongJob := apply
	wrongJob.JobID = "other-job"
	if err := lease.CheckLockedIngress(ctx, wrongJob); err == nil {
		t.Fatal("lease accepted a different signed job")
	}
	retire := apply
	retire.Action, retire.JobID = "public_ingress_retire", "retire-mint-2"
	if err := lease.CheckLockedIngress(ctx, retire); err == nil {
		t.Fatal("Apply lease authorised Retire")
	}
	// Uncertain state revokes the Apply lease (restart/uncertain fencing).
	if err := g.RecordReconciliationNeeded(ctx, apply); err != nil {
		t.Fatal(err)
	}
	if err := lease.CheckLockedIngress(ctx, apply); err == nil {
		t.Fatal("Apply lease survived NEEDS_RECONCILIATION")
	}
	if err := g.RecordCommandIntent(ctx, retire); err != nil {
		t.Fatal(err)
	}
	retireEv := signedEvidence(t, retire)
	rl, err := g.MintLease(ctx, retireEv, retire)
	if err != nil {
		t.Fatalf("Retire from uncertain state with new signed job refused: %v", err)
	}
	if err := rl.CheckLockedIngress(ctx, retire); err != nil {
		t.Fatal(err)
	}
	// Guard Close racing a leaf effect must wait for the effect to finish:
	// the authority check and the effect are one critical section.
	closeDone := make(chan error, 1)
	effectRan := false
	if err := rl.WithLockedIngress(ctx, retire, func() error {
		effectRan = true
		if g.mu.TryLock() {
			g.mu.Unlock()
			return errors.New("guard mutex not held during effect")
		}
		go func() { closeDone <- g.Close() }()
		select {
		case <-closeDone:
			return errors.New("guard Close interleaved with effect")
		case <-time.After(50 * time.Millisecond):
		}
		return nil
	}); err != nil || !effectRan {
		t.Fatalf("live lease effect: ran=%v err=%v", effectRan, err)
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
	effectRan = false
	if err := rl.WithLockedIngress(ctx, retire, func() error { effectRan = true; return nil }); err == nil || effectRan {
		t.Fatalf("effect ran after guard Close: ran=%v err=%v", effectRan, err)
	}
	if err := rl.CheckLockedIngress(ctx, retire); err == nil {
		t.Fatal("lease survived guard Close")
	}
	// Restart: a new holder can never re-mint a consumed signed job.
	g2, err := store.HoldPending(b)
	if err != nil {
		t.Fatal(err)
	}
	defer g2.Close()
	if _, err := g2.MintLease(ctx, ev, apply); err == nil {
		t.Fatal("restarted guard re-minted consumed Apply")
	}
	if _, err := g2.MintLease(ctx, retireEv, retire); !errors.Is(err, ErrLeaseReplay) {
		t.Fatalf("restart replay of Retire not refused: %v", err)
	}
	expired := apply
	expired.ExpiresAt = time.Now().Add(-time.Second)
	if err := lease.CheckLockedIngress(ctx, expired); err == nil {
		t.Fatal("expired command accepted")
	}
}
