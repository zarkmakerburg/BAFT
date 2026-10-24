//go:build unix

package ingressauthority

import (
	"context"
	"time"
)

// CheckHeldCommand revalidates immutable signed-command correlation, the
// protected installed policy and the exact persistent claim while the same
// PendingGuard OS lock remains held. This is an inert recovery/admission check,
// NOT a live Lease or permission for host effects. Positive execution still
// requires independently complete and enforced Delta coverage and a concrete
// lock-bound operational authority.
func (g *PendingGuard) CheckHeldCommand(ctx context.Context, command CommandV1) error {
	if g == nil {
		return ErrUnverified
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.checkPolicyPendingLocked(ctx); err != nil {
		return err
	}
	if err := command.Binding.MatchReviewed(g.binding); err != nil {
		return err
	}
	if err := command.Validate(time.Now()); err != nil {
		return err
	}
	policy, err := g.observeInstalledPolicyLocked(ctx)
	if err != nil {
		return err
	}
	records, err := readLifecycle(g.root)
	if err != nil {
		return err
	}
	entry, ok := records[reservationKey(g.binding)]
	if !ok || entry.Binding != g.binding || entry.Policy == nil || *entry.Policy != policy {
		return ErrUnverified
	}
	stored, ok := entry.Commands[command.Action]
	if !ok || !sameCommandIntent(stored, command) {
		return ErrUnverified
	}
	if entry.State != StateApplying && entry.State != StateNeedsReconciliation {
		return ErrUnverified
	}
	return g.checkPolicyPendingLocked(ctx)
}

// RecordReconciliationNeeded durably records an uncertain resource outcome
// for the exact, immutable command under the held OS lock and pinned policy.
// It never frees a port, asserts retirement, or turns observations into a
// transferable operational permit. Only independently verified exact-owned
// runtime and firewall cleanup could support a future release transition.
func (g *PendingGuard) RecordReconciliationNeeded(ctx context.Context, command CommandV1) error {
	if g == nil {
		return ErrUnverified
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.checkPolicyPendingLocked(ctx); err != nil {
		return err
	}
	if err := command.Binding.MatchReviewed(g.binding); err != nil {
		return err
	}
	if err := command.Validate(time.Now()); err != nil {
		return err
	}
	policy, err := g.observeInstalledPolicyLocked(ctx)
	if err != nil {
		return err
	}
	entries, err := readLifecycle(g.root)
	if err != nil {
		return err
	}
	key := reservationKey(g.binding)
	entry, ok := entries[key]
	if !ok || entry.Binding != g.binding || entry.Policy == nil || *entry.Policy != policy {
		return ErrUnverified
	}
	recorded, ok := entry.Commands[command.Action]
	if !ok || !sameCommandIntent(recorded, command) {
		return ErrUnverified
	}
	entry.State = StateNeedsReconciliation
	entries[key] = entry
	if err := writeLifecycle(g.fd, g.root, entries); err != nil {
		return err
	}
	return g.checkPolicyPendingLocked(ctx)
}
