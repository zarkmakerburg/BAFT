package ingressownership

import (
	"errors"
	"testing"
)

func TestLABJournalTransitionsAndGenerationFence(t *testing.T) {
	j := new(LabJournal)
	p := proof("journal", 1)
	record := MutationRecord{Port: 443, Owner: p, Phase: PhaseIntent}
	if err := j.Advance(record); err != nil {
		t.Fatal(err)
	}
	if err := j.Advance(record); err != nil {
		t.Fatal(err)
	}
	if err := j.Advance(MutationRecord{Port: 2053, Owner: p, Phase: PhaseListenerBound}); !errors.Is(err, ErrConflict) {
		t.Fatalf("port changed: %v", err)
	}
	if err := j.Advance(MutationRecord{Port: 443, Owner: p, Phase: PhaseVerified}); err == nil {
		t.Fatal("skipped verification phases")
	}
	for _, phase := range []MutationPhase{PhaseListenerBound, PhaseFirewallInstalled, PhaseVerified, PhaseRetireIntent, PhaseRetired} {
		record.Phase = phase
		if err := j.Advance(record); err != nil {
			t.Fatalf("%s: %v", phase, err)
		}
	}
	next := MutationRecord{Port: 443, Owner: proof("journal", 2), Phase: PhaseIntent}
	if err := j.Advance(next); err != nil {
		t.Fatal(err)
	}
	if err := j.Advance(record); !errors.Is(err, ErrStale) {
		t.Fatalf("stale generation accepted: %v", err)
	}
	if err := j.Advance(MutationRecord{Port: 22, Owner: proof("ssh", 1), Phase: PhaseIntent}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("SSH ownership accepted: %v", err)
	}
	if len(j.Snapshot()) != 1 {
		t.Fatal("unexpected journal entries")
	}
}

func TestLABJournalAdvanceRejectsPrematureFirewallHandle(t *testing.T) {
	for _, phase := range []MutationPhase{PhaseIntent, PhaseListenerBound} {
		j := new(LabJournal)
		record := MutationRecord{Port: 443, Owner: proof("journal-handle-fence", 1), Phase: phase, FirewallHandle: 42}
		if phase == PhaseListenerBound {
			if err := j.Advance(MutationRecord{Port: 443, Owner: record.Owner, Phase: PhaseIntent}); err != nil {
				t.Fatal(err)
			}
		}
		if err := j.Advance(record); !errors.Is(err, ErrInvalid) {
			t.Fatalf("phase %s accepted premature handle: %v", phase, err)
		}
	}
}
