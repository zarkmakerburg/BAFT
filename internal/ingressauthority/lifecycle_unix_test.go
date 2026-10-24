//go:build unix

package ingressauthority

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDurableLifecycleRestartRetainsUnknownAndPendingClaim(t *testing.T) {
	dir := privateReservationDir(t)
	store, err := OpenReservationStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	b := bindingFixture(t)
	if err := store.ReservePending(b); err != nil {
		t.Fatal(err)
	}
	g, err := store.HoldPending(b)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.MarkApplying(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := OpenReservationStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	state, err := restarted.InspectRecovery(b)
	if err != nil {
		t.Fatal(err)
	}
	if state != StateNeedsReconciliation {
		t.Fatalf("unfinished apply wrongly recovered as ready: %s", state)
	}
	if err := restarted.ReservePending(b); err != nil {
		t.Fatal(err)
	}
	other := b
	other.LeaseID = "new-lease"
	if err := restarted.ReservePending(other); err == nil {
		t.Fatal("unresolved claim stolen")
	}
}

func TestLifecycleCannotAdvanceAfterGuardClose(t *testing.T) {
	dir := privateReservationDir(t)
	store, err := OpenReservationStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	b := bindingFixture(t)
	if err := store.ReservePending(b); err != nil {
		t.Fatal(err)
	}
	g, err := store.HoldPending(b)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if err := g.MarkApplying(context.Background(), b); err == nil {
		t.Fatal("closed guard wrote state")
	}
	state, err := store.InspectRecovery(b)
	if err != nil {
		t.Fatal(err)
	}
	if state != StateNeedsReconciliation {
		t.Fatalf("missing state not conservative: %s", state)
	}
}

func TestLifecycleCorruptAndUnknownClaimsDenyRecovery(t *testing.T) {
	dir := privateReservationDir(t)
	s, err := OpenReservationStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	b := bindingFixture(t)
	if err := s.ReservePending(b); err != nil {
		t.Fatal(err)
	}
	unknown := b
	unknown.LeaseID = "unknown-lease"
	if _, err := s.InspectRecovery(unknown); err == nil {
		t.Fatal("foreign lease recovered")
	}
	if err := os.WriteFile(filepath.Join(dir, lifecycleFile), []byte("null"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InspectRecovery(b); err == nil {
		t.Fatal("corrupt journal recovered as available")
	}
	if err := s.ReservePending(unknown); err == nil {
		t.Fatal("conflicting reservation accepted")
	}
}

func TestLifecycleCrashBeforeJournalRecordNeedsReconciliation(t *testing.T) {
	dir := privateReservationDir(t)
	s, err := OpenReservationStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	b := bindingFixture(t)
	if err := s.ReservePending(b); err != nil {
		t.Fatal(err)
	}
	restarted, err := OpenReservationStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	state, err := restarted.InspectRecovery(b)
	if err != nil || state != StateNeedsReconciliation {
		t.Fatalf("crash gap: state=%v err=%v", state, err)
	}
}
