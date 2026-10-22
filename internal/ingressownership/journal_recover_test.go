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
