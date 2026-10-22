package ingressownership

import (
	"errors"
	"testing"
)

func TestLABJournalPortOwnershipAcrossInstances(t *testing.T) {
	a := MutationRecord{Port: 443, Owner: proof("first", 1), Phase: PhaseIntent}
	b := MutationRecord{Port: 443, Owner: proof("second", 1), Phase: PhaseIntent}
	j := new(LabJournal)
	if err := j.Advance(a); err != nil {
		t.Fatal(err)
	}
	if err := j.Advance(b); !errors.Is(err, ErrConflict) {
		t.Fatalf("cross-owner port collision accepted: %v", err)
	}
	if _, _, err := RecoverLabJournal([]MutationRecord{a, b}); !errors.Is(err, ErrConflict) {
		t.Fatalf("restart collision accepted: %v", err)
	}
	for _, phase := range []MutationPhase{PhaseListenerBound, PhaseFirewallInstalled, PhaseVerified, PhaseRetireIntent, PhaseRetired} {
		a.Phase = phase
		if err := j.Advance(a); err != nil {
			t.Fatal(err)
		}
	}
	if err := j.Advance(b); err != nil {
		t.Fatalf("retired port not reusable: %v", err)
	}
}
