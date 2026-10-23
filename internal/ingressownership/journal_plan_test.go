package ingressownership

import (
	"errors"
	"testing"
)

func TestLABJournalReconciliationPlanDoesNotAdoptActiveResources(t *testing.T) {
	active := MutationRecord{Port: 443, Owner: proof("active", 1), Phase: PhaseVerified}
	retired := MutationRecord{Port: 2053, Owner: proof("retired", 1), Phase: PhaseRetired}
	plan, err := PlanJournalReconciliation([]MutationRecord{retired, active})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Unverified) != 1 || plan.Unverified[0] != active || len(plan.Retired) != 1 || plan.Retired[0] != retired {
		t.Fatalf("incorrect reconciliation plan: %+v", plan)
	}
	if _, err := PlanJournalReconciliation([]MutationRecord{active, active}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate accepted: %v", err)
	}
}
