package ingressownership

import (
	"errors"
	"sync"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// MutationPhase is the LAB journal state machine. A phase alone never
// authorizes adopting a listener or firewall after restart.
type MutationPhase string

const (
	PhaseIntent            MutationPhase = "intent"
	PhaseListenerBound     MutationPhase = "listener_bound"
	PhaseFirewallInstalled MutationPhase = "firewall_installed"
	PhaseVerified          MutationPhase = "verified"
	PhaseRetireIntent      MutationPhase = "retire_intent" // also cleanup of a partially installed operation; not proof of a verified Apply
	PhaseRetired           MutationPhase = "retired"
)

type MutationRecord struct {
	Port           uint16
	Owner          ingressmanifest.Proof
	Phase          MutationPhase
	FirewallHandle uint64 // zero for the memory firewall; pinned for nft LAB
}

// MutationJournal exposes phase persistence to the isolated transaction.
// A current phase is never proof that a host resource is still owned.
type MutationJournal interface {
	Advance(MutationRecord) error
	Current(ingressmanifest.Proof) (MutationRecord, bool, error)
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
	if (next.Phase == PhaseIntent || next.Phase == PhaseListenerBound) && next.FirewallHandle != 0 {
		return ErrInvalid
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.entries == nil {
		j.entries = make(map[string]MutationRecord)
	}
	key := journalKey(next.Owner)
	// A different owner may not claim a live or uncertain public port.
	// Retired entries no longer reserve the port.
	for otherKey, entry := range j.entries {
		if otherKey != key && entry.Port == next.Port && entry.Phase != PhaseRetired {
			return ErrConflict
		}
	}
	old, ok := j.entries[key]
	if !ok {
		if next.Phase != PhaseIntent {
			return ErrUnverified
		}
		j.entries[key] = next
		return nil
	}
	// A retired generation must never be reopened by replaying its intent.
	if old.Phase == PhaseRetired && next.Owner.Generation == old.Owner.Generation && next.Phase != PhaseRetired {
		return ErrConflict
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
	if (old.FirewallHandle != 0 && next.FirewallHandle != old.FirewallHandle) || (old.FirewallHandle == 0 && next.FirewallHandle != 0 && (old.Phase != PhaseListenerBound || (next.Phase != PhaseFirewallInstalled && next.Phase != PhaseRetireIntent))) {
		return ErrConflict
	}
	if next.Phase == old.Phase {
		return nil
	}
	allowed := map[MutationPhase]MutationPhase{PhaseIntent: PhaseListenerBound, PhaseListenerBound: PhaseFirewallInstalled, PhaseFirewallInstalled: PhaseVerified, PhaseVerified: PhaseRetireIntent, PhaseRetireIntent: PhaseRetired}
	rollbackIntent := (old.Phase == PhaseListenerBound || old.Phase == PhaseFirewallInstalled) && next.Phase == PhaseRetireIntent && next.FirewallHandle != 0
	if allowed[old.Phase] != next.Phase && !rollbackIntent {
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

func (j *LabJournal) Current(owner ingressmanifest.Proof) (MutationRecord, bool, error) {
	if j == nil {
		return MutationRecord{}, false, ErrInvalid
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	r, ok := j.entries[journalKey(owner)]
	return r, ok, nil
}
