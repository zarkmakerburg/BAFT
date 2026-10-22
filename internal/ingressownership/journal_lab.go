package ingressownership

import (
	"errors"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"sync"
)

// MutationPhase is the LAB journal state machine. A phase alone never
// authorizes adopting a listener or firewall after restart.
type MutationPhase string

const (
	PhaseIntent            MutationPhase = "intent"
	PhaseListenerBound     MutationPhase = "listener_bound"
	PhaseFirewallInstalled MutationPhase = "firewall_installed"
	PhaseVerified          MutationPhase = "verified"
	PhaseRetired           MutationPhase = "retired"
)

type MutationRecord struct {
	Port  uint16
	Owner ingressmanifest.Proof
	Phase MutationPhase
}

// LabJournal tracks monotonic transitions and rejects ambiguous replays.
// It is deliberately memory-only until a durable transactional store exists.
type LabJournal struct {
	mu      sync.Mutex
	entries map[string]MutationRecord
}

func journalKey(p ingressmanifest.Proof) string { return p.TunnelID + "\x00" + p.InstanceID }
func (j *LabJournal) Advance(next MutationRecord) error {
	if j == nil {
		return ErrInvalid
	}
	if _, err := PlanIngressRule(next.Port, next.Owner); err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.entries == nil {
		j.entries = make(map[string]MutationRecord)
	}
	key := journalKey(next.Owner)
	old, ok := j.entries[key]
	if !ok {
		if next.Phase != PhaseIntent {
			return ErrUnverified
		}
		j.entries[key] = next
		return nil
	}
	if next.Owner.Generation < old.Owner.Generation {
		return ErrStale
	}
	if next.Owner.Generation > old.Owner.Generation {
		if old.Phase != PhaseRetired || next.Phase != PhaseIntent {
			return ErrConflict
		}
		j.entries[key] = next
		return nil
	}
	if next.Port != old.Port || next.Owner != old.Owner {
		return ErrConflict
	}
	if next.Phase == old.Phase {
		return nil
	}
	allowed := map[MutationPhase]MutationPhase{PhaseIntent: PhaseListenerBound, PhaseListenerBound: PhaseFirewallInstalled, PhaseFirewallInstalled: PhaseVerified, PhaseVerified: PhaseRetired}
	if allowed[old.Phase] != next.Phase {
		return errors.Join(ErrInvalid, ErrUnverified)
	}
	j.entries[key] = next
	return nil
}
func (j *LabJournal) Snapshot() []MutationRecord {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]MutationRecord, 0, len(j.entries))
	for _, v := range j.entries {
		out = append(out, v)
	}
	return out
}
