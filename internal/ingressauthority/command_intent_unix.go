//go:build unix

package ingressauthority

import (
	"context"
	"os"
	"time"
)

const commandIntentApply = "public_ingress_apply"
const commandIntentRetire = "public_ingress_retire"

func sameCommandIntent(a, b CommandV1) bool {
	return a.IngressProtocol == b.IngressProtocol && a.JobID == b.JobID && a.Action == b.Action && a.Binding == b.Binding && a.IssuedAt.Equal(b.IssuedAt) && a.ExpiresAt.Equal(b.ExpiresAt)
}

// mayResumeRetire reports whether command is a NEW signed Retire that may
// replace old, a Retire intent whose lease was already consumed while the
// operation is still NEEDS_RECONCILIATION (cleanup incomplete). Nothing else
// may replace an intent.
func mayResumeRetire(entry lifecycleEntry, old, command CommandV1) bool {
	return command.Action == commandIntentRetire && old.Action == commandIntentRetire &&
		entry.State == StateNeedsReconciliation && entry.Leased[commandIntentRetire] == old.JobID &&
		old.JobID != command.JobID && command.JobID != "" && old.Binding == command.Binding
}

func validCommandIntents(entry lifecycleEntry) bool {
	if len(entry.Commands) > 2 || (len(entry.Commands) > 0 && entry.Policy == nil) {
		return false
	}
	for action, cmd := range entry.Commands {
		if action != cmd.Action || (action != commandIntentApply && action != commandIntentRetire) || cmd.Binding != entry.Binding || cmd.Validate(cmd.IssuedAt) != nil {
			return false
		}
	}
	if len(entry.Leased) > 2 || len(entry.Superseded) > maxSupersededRetires {
		return false
	}
	seen := make(map[string]bool)
	for _, spent := range entry.Superseded {
		if !idPattern.MatchString(spent) || seen[spent] {
			return false
		}
		seen[spent] = true
		for _, cmd := range entry.Commands {
			if cmd.JobID == spent {
				return false
			}
		}
	}
	for action, jobID := range entry.Leased {
		if cmd, ok := entry.Commands[action]; !ok || cmd.JobID != jobID {
			return false
		}
	}
	if retire, ok := entry.Commands[commandIntentRetire]; ok {
		apply, ok := entry.Commands[commandIntentApply]
		if !ok || retire.JobID == apply.JobID {
			return false
		}
	}
	return true
}

// maxSupersededRetires bounds the durable list of spent Retire job IDs.
// Once reached, no further Retire supersession is allowed (fail closed).
const maxSupersededRetires = 32

func jobSpent(entries map[string]lifecycleEntry, jobID string) bool {
	for _, record := range entries {
		for _, spent := range record.Superseded {
			if spent == jobID {
				return true
			}
		}
		for _, old := range record.Commands {
			if old.JobID == jobID {
				return true
			}
		}
	}
	return false
}

// planCommandRecord is the pure admission decision for recording command.
// same reports an exact retry of the stored intent (nothing to write). A job
// ID that was ever recorded, including one superseded by a later Retire, can
// never be recorded again, so two signed Retires cannot ping-pong.
func planCommandRecord(entries map[string]lifecycleEntry, entry lifecycleEntry, command CommandV1, now time.Time) (lifecycleEntry, bool, error) {
	superseding := false
	if old, ok := entry.Commands[command.Action]; ok {
		if sameCommandIntent(old, command) {
			return entry, true, nil
		}
		// Per-action identity is immutable EXCEPT for resuming a Retire whose
		// previous attempt already consumed its lease and left the operation
		// in NEEDS_RECONCILIATION (cleanup incomplete): a NEW, separately
		// identified signed Retire may then replace the spent intent. An
		// unspent Retire intent, an Apply intent, or any other state stays
		// immutable. The spent JobID is remembered durably (Superseded) and
		// can never be recorded or leased again.
		if !mayResumeRetire(entry, old, command) || len(entry.Superseded) >= maxSupersededRetires {
			return entry, false, ErrBinding
		}
		superseding = true
	}
	if err := command.Validate(now); err != nil {
		return entry, false, err
	}
	if jobSpent(entries, command.JobID) {
		return entry, false, ErrBinding
	}
	// Copy maps: never mutate the caller's decoded state on a refusal.
	cmds := make(map[string]CommandV1, len(entry.Commands)+1)
	for k, v := range entry.Commands {
		cmds[k] = v
	}
	leased := make(map[string]string, len(entry.Leased))
	for k, v := range entry.Leased {
		leased[k] = v
	}
	superseded := append([]string(nil), entry.Superseded...)
	if superseding {
		superseded = append(superseded, cmds[commandIntentRetire].JobID)
		delete(leased, commandIntentRetire)
	}
	command.IssuedAt = command.IssuedAt.UTC()
	command.ExpiresAt = command.ExpiresAt.UTC()
	cmds[command.Action] = command
	entry.Commands, entry.Superseded = cmds, superseded
	if len(leased) == 0 {
		leased = nil
	}
	entry.Leased = leased
	if len(superseded) == 0 {
		entry.Superseded = nil
	}
	if !validCommandIntents(entry) {
		return entry, false, ErrBinding
	}
	return entry, false, nil
}

// RecordCommandIntent records data under an existing held OS guard, exact
// pending binding and independently pinned policy. It does NOT authenticate
// a signature, authorize host effects, mint Lease or claim readiness. Normal
// Agent admission must independently verify the signed job before invoking it.
// Per-action identity is immutable; Retire needs a separately identified job
// for the same resource and an existing Apply intent. An exact retry may be
// read back after expiry, but can never renew a runnable command or lease.
func (g *PendingGuard) RecordCommandIntent(ctx context.Context, command CommandV1) error {
	if g == nil {
		return ErrUnverified
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.checkPolicyPendingLocked(ctx); err != nil {
		return err
	}
	if command.Action != commandIntentApply && command.Action != commandIntentRetire {
		return ErrBinding
	}
	if err := command.Binding.MatchReviewed(g.binding); err != nil {
		return err
	}
	if err := command.Validate(command.IssuedAt); err != nil {
		return err
	}
	entries, err := readLifecycle(g.root)
	if err != nil {
		return err
	}
	// A JobID spent by an earlier, released reservation never runs again.
	released, err := loadReleased(g.root)
	if err != nil {
		return err
	}
	if releasedSpent(released, command.JobID) {
		return ErrBinding
	}
	key := reservationKey(g.binding)
	entry, ok := entries[key]
	if !ok || entry.Binding != g.binding || entry.Policy == nil {
		return ErrUnverified
	}
	observed, err := g.observeInstalledPolicyLocked(ctx)
	if err != nil {
		return err
	}
	if *entry.Policy != observed {
		return ErrUnverified
	}
	entry, same, err := planCommandRecord(entries, entry, command, time.Now())
	if err != nil {
		return err
	}
	if same {
		return nil
	}
	entries[key] = entry
	if err := writeLifecycle(g.fd, g.root, entries); err != nil {
		return err
	}
	return g.checkPolicyPendingLocked(ctx)
}

// InspectCommandIntents returns detached correlation data for recovery, even
// if the installed policy has since drifted. It never declares operational
// readiness or removes a reservation. A held mutation guard blocks readback.
func (s *ReservationStore) InspectCommandIntents(b BindingV1) ([]CommandV1, error) {
	if s == nil {
		return nil, ErrUnverified
	}
	if err := b.Validate(); err != nil {
		return nil, err
	}
	var result []CommandV1
	err := s.withLock(func(_ *os.File, root *os.Root) error {
		claims, err := loadPending(root)
		if err != nil {
			return err
		}
		if current, ok := claims[reservationKey(b)]; !ok || current != b {
			return ErrUnverified
		}
		records, err := readLifecycle(root)
		if err != nil {
			return err
		}
		record, ok := records[reservationKey(b)]
		if !ok {
			return nil
		}
		if record.Binding != b {
			return ErrUnverified
		}
		for _, action := range []string{commandIntentApply, commandIntentRetire} {
			if cmd, ok := record.Commands[action]; ok {
				result = append(result, cmd)
			}
		}
		return nil
	})
	return result, err
}
