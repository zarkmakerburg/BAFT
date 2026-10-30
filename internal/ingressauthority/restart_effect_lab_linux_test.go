//go:build linux

package ingressauthority

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRestartAuthorityLABCloseWaitsForReconciliationEffect(t *testing.T) {
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
	guard, err := store.HoldPending(b)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = guard.Close() })
	if _, err := guard.BindInstalledPolicy(ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	command := CommandV1{IngressProtocol: ProtocolV1, JobID: "restart-effect-1", Action: commandIntentApply,
		IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Binding: b}
	if err := guard.RecordCommandIntent(ctx, command); err != nil {
		t.Fatal(err)
	}
	if err := guard.MarkApplying(ctx, b); err != nil {
		t.Fatal(err)
	}
	if _, err := guard.MintLease(ctx, signedEvidence(t, command), command); err != nil {
		t.Fatal(err)
	}
	authority, err := guard.RestartAuthority(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ran := false
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := authority.WithLockedReconciliation(cancelled, func() error { ran = true; return nil }); err == nil || ran {
		t.Fatal("cancelled reconciliation ran an effect")
	}
	if err := authority.WithLockedReconciliation(ctx, nil); !errors.Is(err, ErrUnverified) {
		t.Fatalf("nil effect: %v", err)
	}
	closeStarted := make(chan struct{})
	closeDone := make(chan error, 1)
	if err := authority.WithLockedReconciliation(ctx, func() error {
		ran = true
		if guard.mu.TryLock() {
			guard.mu.Unlock()
			return errors.New("guard mutex was not retained across reconciliation effect")
		}
		go func() {
			close(closeStarted)
			closeDone <- guard.Close()
		}()
		<-closeStarted
		select {
		case <-closeDone:
			return errors.New("guard Close interleaved with reconciliation effect")
		case <-time.After(25 * time.Millisecond):
		}
		return nil
	}); err != nil || !ran {
		t.Fatalf("held restart effect: ran=%v err=%v", ran, err)
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
	ran = false
	if err := authority.WithLockedReconciliation(ctx, func() error { ran = true; return nil }); err == nil || ran {
		t.Fatal("reconciliation effect ran after guard Close")
	}
}
