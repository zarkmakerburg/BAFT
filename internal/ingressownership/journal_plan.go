package ingressownership

import "sort"

// JournalPlan is an inert snapshot of the work needed after restart. It
// never permits automatic adoption or destructive cleanup of host resources.
type JournalPlan struct {
	Unverified []MutationRecord
	Retired    []MutationRecord
}

// PlanJournalReconciliation deterministically partitions validated journal
// entries. Callers must obtain fresh independent kernel and socket evidence.
func PlanJournalReconciliation(records []MutationRecord) (JournalPlan, error) {
	_, recovery, err := RecoverLabJournal(records)
	if err != nil {
		return JournalPlan{}, err
	}
	plan := JournalPlan{Unverified: recovery.NeedsReconciliation}
	for _, r := range recovery.Records {
		if r.Phase == PhaseRetired {
			plan.Retired = append(plan.Retired, r)
		}
	}
	sort.Slice(plan.Retired, func(i, j int) bool { return journalKey(plan.Retired[i].Owner) < journalKey(plan.Retired[j].Owner) })
	return plan, nil
}
