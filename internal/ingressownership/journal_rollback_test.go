package ingressownership

import "testing"

func TestJournalPartialCleanupIntentPinsHandleWithoutVerifiedApply(t *testing.T) {
	for _, installed := range []bool{false, true} {
		j := new(LabJournal)
		owner := proof("partial-cleanup-intent", 1)
		for _, phase := range []MutationPhase{PhaseIntent, PhaseListenerBound} {
			if err := j.Advance(MutationRecord{Port: 54618, Owner: owner, Phase: phase}); err != nil {
				t.Fatal(err)
			}
		}
		if installed {
			if err := j.Advance(MutationRecord{Port: 54618, Owner: owner, Phase: PhaseFirewallInstalled, FirewallHandle: 42}); err != nil {
				t.Fatal(err)
			}
		}
		for _, handle := range []uint64{0, 43} {
			if !installed && handle == 43 {
				continue
			} // A newly observed handle is allowed only from the unpinned phase.
			if err := j.Advance(MutationRecord{Port: 54618, Owner: owner, Phase: PhaseRetireIntent, FirewallHandle: handle}); err == nil {
				t.Fatalf("accepted missing or changed cleanup handle %d", handle)
			}
		}
		if err := j.Advance(MutationRecord{Port: 54618, Owner: owner, Phase: PhaseRetireIntent, FirewallHandle: 42}); err != nil {
			t.Fatal(err)
		}
		_, recovery, err := RecoverLabJournal(j.Snapshot())
		if err != nil || len(recovery.NeedsReconciliation) != 1 || recovery.NeedsReconciliation[0].Phase != PhaseRetireIntent {
			t.Fatalf("cleanup intent adopted live ownership: %+v %v", recovery, err)
		}
		if err := j.Advance(MutationRecord{Port: 54618, Owner: owner, Phase: PhaseRetired, FirewallHandle: 42}); err != nil {
			t.Fatal(err)
		}
	}
}
