//go:build unix

package ingressauthority

import (
	"context"
	"errors"

	"github.com/zarkmakerburg/baft/internal/agentjob"
)

// ErrLeaseReplay means a live lease for this signed job was already minted
// (possibly by a prior process). It is single-use and never re-minted.
var ErrLeaseReplay = errors.New("ingress lease already minted for signed job")

// heldLease binds one immutable command to the still-held PendingGuard.
type heldLease struct {
	guard   *PendingGuard
	command CommandV1
}

// MintLease returns the only kind of valid *Lease. It requires sealed
// agentjob.VerifiedJob evidence (a locally enrolled signature/node/window
// check), the exact command, and this open guard's OS lock, durable pending
// claim, pinned installed policy, recorded command intent and lifecycle state.
// Apply requires APPLYING_UNVERIFIED; Retire may also run from
// NEEDS_RECONCILIATION. Each signed job mints at most once, durably; restart
// or replay is refused. The lease is revoked by guard Close, expiry, policy or
// claim drift, or (for Apply) a recorded uncertain state.
func (g *PendingGuard) MintLease(ctx context.Context, job agentjob.VerifiedJob, command CommandV1) (*Lease, error) {
	if g == nil || !job.Valid() {
		return nil, ErrUnverified
	}
	j := job.Job()
	if j.JobID != command.JobID || j.NodeID != command.Binding.NodeID || j.Action != command.Action ||
		!j.IssuedAt.Equal(command.IssuedAt) || !j.ExpiresAt.Equal(command.ExpiresAt) {
		return nil, ErrBinding
	}
	if command.Action != commandIntentApply && command.Action != commandIntentRetire {
		return nil, ErrBinding
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	records, entry, err := g.checkHeldCommandLocked(ctx, command)
	if err != nil {
		return nil, err
	}
	if err := leaseStateOK(entry, command.Action); err != nil {
		return nil, err
	}
	if _, used := entry.Leased[command.Action]; used {
		return nil, ErrLeaseReplay
	}
	if entry.Leased == nil {
		entry.Leased = make(map[string]string)
	}
	entry.Leased[command.Action] = command.JobID
	records[reservationKey(g.binding)] = entry
	// Durable consume BEFORE the lease exists: a crash cannot yield a replay.
	if err := writeLifecycle(g.fd, g.root, records); err != nil {
		return nil, err
	}
	if err := g.checkPolicyPendingLocked(ctx); err != nil {
		return nil, err
	}
	return &Lease{minted: true, binding: command.Binding, authority: &heldLease{guard: g, command: command}}, nil
}

func leaseStateOK(entry lifecycleEntry, action string) error {
	switch {
	case action == commandIntentApply && entry.State == StateApplying:
		return nil
	case action == commandIntentRetire && (entry.State == StateApplying || entry.State == StateNeedsReconciliation):
		return nil
	}
	return ErrUnverified
}

func (h *heldLease) checkLease(ctx context.Context, command CommandV1) error {
	if h == nil || h.guard == nil || !sameCommandIntent(h.command, command) {
		return ErrUnverified
	}
	h.guard.mu.Lock()
	defer h.guard.mu.Unlock()
	return h.checkLeaseLocked(ctx, command)
}

// withLease holds the guard mutex across the final check, the effect and a
// post-effect recheck, so guard Close cannot release the OS lock mid-effect.
func (h *heldLease) withLease(ctx context.Context, command CommandV1, effect func() error) error {
	if h == nil || h.guard == nil || !sameCommandIntent(h.command, command) {
		return ErrUnverified
	}
	h.guard.mu.Lock()
	defer h.guard.mu.Unlock()
	if err := h.checkLeaseLocked(ctx, command); err != nil {
		return err
	}
	err := effect()
	return errors.Join(err, h.checkLeaseLocked(ctx, command))
}

func (h *heldLease) checkLeaseLocked(ctx context.Context, command CommandV1) error {
	_, entry, err := h.guard.checkHeldCommandLocked(ctx, command)
	if err != nil {
		return err
	}
	if entry.Leased[command.Action] != command.JobID {
		return ErrUnverified
	}
	return leaseStateOK(entry, command.Action)
}
