package ingressownership

import (
	"errors"
	"testing"
)

func TestLABJournalRestartRequiresIndependentReconciliation(t *testing.T) {
	p := proof("restart", 4)
	record := MutationRecord{Port: 443, Owner: p, Phase: PhaseVerified}
	j, recovery, err := RecoverLabJournal([]MutationRecord{record})
	if err != nil {
		t.Fatal(err)
	}
	if len(j.Snapshot()) != 1 || len(recovery.NeedsReconciliation) != 1 {
		t.Fatal("lost unverified restart intent")
	}
	if err := ConfirmLabRecovery(p, recovery); !errors.Is(err, ErrUnverified) {
		t.Fatalf("active resource auto-adopted: %v", err)
	}
	if _, _, err := RecoverLabJournal([]MutationRecord{record, record}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate accepted: %v", err)
	}
	record.Phase = "unknown"
	if _, _, err := RecoverLabJournal([]MutationRecord{record}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid phase accepted: %v", err)
	}
}

func TestRecoveryRejectsPrematureFirewallHandle(t *testing.T) {
	for _, phase := range []MutationPhase{PhaseIntent, PhaseListenerBound} {
		record := MutationRecord{Port: 443, Owner: proof("premature-handle", 1), Phase: phase, FirewallHandle: 42}
		if _, _, err := RecoverLabJournal([]MutationRecord{record}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("phase %s accepted impossible handle: %v", phase, err)
		}
	}
}

func TestRetiredGenerationCannotResurrectOnReplay(t *testing.T) {
	p := proof("retired-replay", 7)
	j := new(LabJournal)
	for _, phase := range []MutationPhase{PhaseIntent, PhaseListenerBound, PhaseFirewallInstalled, PhaseVerified, PhaseRetireIntent, PhaseRetired} {
		if err := j.Advance(MutationRecord{Port: 443, Owner: p, Phase: phase}); err != nil {
			t.Fatalf("advance %s: %v", phase, err)
		}
	}
	if err := j.Advance(MutationRecord{Port: 443, Owner: p, Phase: PhaseIntent}); !errors.Is(err, ErrConflict) {
		t.Fatalf("retired generation resurrected: %v", err)
	}
	r, ok, err := j.Current(p)
	if err != nil || !ok || r.Phase != PhaseRetired {
		t.Fatalf("retired journal mutated by replay: %+v, %v", r, err)
	}
}
