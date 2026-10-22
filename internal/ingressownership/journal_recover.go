package ingressownership

import (
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"sort"
)

// JournalRecovery is a fail-closed, data-only restart assessment. An active
// entry remains unverified even if its last recorded phase was verified.
type JournalRecovery struct {
	Records             []MutationRecord
	NeedsReconciliation []MutationRecord
}

// RecoverLabJournal validates a snapshot without adopting listeners, firewall
// rules or permission to perform any mutation. Incomplete entries remain
// visible for explicit operator-driven reconciliation.
func RecoverLabJournal(records []MutationRecord) (*LabJournal, JournalRecovery, error) {
	journal := new(LabJournal)
	seen := make(map[string]bool, len(records))
	result := JournalRecovery{}
	for _, r := range records {
		if _, err := PlanIngressRule(r.Port, r.Owner); err != nil {
			return nil, JournalRecovery{}, err
		}
		k := journalKey(r.Owner)
		if seen[k] {
			return nil, JournalRecovery{}, ErrConflict
		}
		seen[k] = true
		switch r.Phase {
		case PhaseIntent, PhaseListenerBound, PhaseFirewallInstalled, PhaseVerified, PhaseRetired:
		default:
			return nil, JournalRecovery{}, ErrInvalid
		}
		if journal.entries == nil {
			journal.entries = make(map[string]MutationRecord)
		}
		journal.entries[k] = r
		result.Records = append(result.Records, r)
		if r.Phase != PhaseRetired {
			result.NeedsReconciliation = append(result.NeedsReconciliation, r)
		}
	}
	sort.Slice(result.Records, func(i, j int) bool { return journalKey(result.Records[i].Owner) < journalKey(result.Records[j].Owner) })
	sort.Slice(result.NeedsReconciliation, func(i, j int) bool {
		return journalKey(result.NeedsReconciliation[i].Owner) < journalKey(result.NeedsReconciliation[j].Owner)
	})
	return journal, result, nil
}

// ConfirmLabRecovery intentionally refuses automatic adoption of recorded
// active resources until independent kernel/socket evidence is integrated.
func ConfirmLabRecovery(_ ingressmanifest.Proof, _ JournalRecovery) error { return ErrUnverified }
