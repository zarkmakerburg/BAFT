//go:build unix

package ingressauthority

import (
	"context"
	"errors"
)

// RestartDirection names the spent signed job whose consumed lease is the
// durable authorization for finishing an interrupted effect after a restart.
type RestartDirection string

const (
	// RestartApply: an Apply lease was minted; the interrupted effect is rolled
	// back (an Apply can never be resumed forward: its job is spent).
	RestartApply RestartDirection = "apply"
	// RestartRetire: a Retire lease was minted; the interrupted retirement is
	// completed.
	RestartRetire RestartDirection = "retire"
)

// RestartAuthority is a REMOVE-ONLY, data-only credential for post-restart
// reconciliation of one binding. It is deliberately not a Lease:
//
//   - It exists only for a binding whose lifecycle durably records that a
//     signed job's lease was ALREADY consumed (Leased). A recorded-but-never-
//     leased intent, an expired unleased command, a superseded (spent) Retire
//     or a bare journal entry never yields one.
//   - It never mints, renews, replaces or clears a lease: the Leased /
//     Commands / Superseded fields are never written by anything reachable
//     from it, so a spent job stays spent (no ping-pong revival).
//   - It carries no capability to bind a socket or add a firewall rule; the
//     only consumer (ingressownership.RestartReconciler) can only delete the
//     exact owner-tagged rule recorded for this binding.
//   - It is bound to the still-held PendingGuard OS lock and re-verifies the
//     pinned installed policy, claim and lifecycle on every Check.
type RestartAuthority struct {
	minted    bool
	guard     *PendingGuard
	binding   BindingV1
	direction RestartDirection
	command   CommandV1
	state     RecoveryState
}

func (a *RestartAuthority) Binding() BindingV1          { return a.binding }
func (a *RestartAuthority) Direction() RestartDirection { return a.direction }
func (a *RestartAuthority) State() RecoveryState        { return a.state }

// Command is the stored, already-leased signed command (historical: it may be
// expired; expiry is irrelevant to a remove-only reconciliation).
func (a *RestartAuthority) Command() CommandV1 { return a.command }

// restartEntryLocked loads and verifies the lifecycle entry under g.mu.
func (g *PendingGuard) restartEntryLocked(ctx context.Context) (lifecycleEntry, error) {
	var zero lifecycleEntry
	if err := g.checkPolicyPendingLocked(ctx); err != nil {
		return zero, err
	}
	policy, err := g.observeInstalledPolicyLocked(ctx)
	if err != nil {
		return zero, err
	}
	entries, err := readLifecycle(g.root)
	if err != nil {
		return zero, err
	}
	entry, ok := entries[reservationKey(g.binding)]
	if !ok || entry.Binding != g.binding || entry.Policy == nil || *entry.Policy != policy {
		return zero, ErrUnverified
	}
	if entry.State != StateApplying && entry.State != StateNeedsReconciliation {
		return zero, ErrUnverified
	}
	if !validCommandIntents(entry) {
		return zero, ErrUnverified
	}
	return entry, nil
}

// restartDirection picks the consumed lease that authorizes the cleanup. A
// Retire lease wins over an Apply lease. The leased job must be the stored
// intent of that action; anything else is unexplained and denies.
func restartDirection(entry lifecycleEntry) (RestartDirection, CommandV1, error) {
	for _, c := range []struct {
		action string
		dir    RestartDirection
	}{{commandIntentRetire, RestartRetire}, {commandIntentApply, RestartApply}} {
		jobID, leased := entry.Leased[c.action]
		if !leased {
			continue
		}
		cmd, ok := entry.Commands[c.action]
		if !ok || cmd.JobID != jobID || cmd.Binding != entry.Binding {
			return "", CommandV1{}, ErrUnverified
		}
		for _, spent := range entry.Superseded {
			if spent == jobID {
				return "", CommandV1{}, ErrUnverified
			}
		}
		return c.dir, cmd, nil
	}
	return "", CommandV1{}, ErrUnverified
}

// RestartAuthority derives the remove-only restart credential for this
// guard's binding. It requires the held OS lock (HoldPending), the exact
// pending claim, the pinned installed policy and a lifecycle entry in
// APPLYING_UNVERIFIED or NEEDS_RECONCILIATION that records a CONSUMED lease.
func (g *PendingGuard) RestartAuthority(ctx context.Context) (*RestartAuthority, error) {
	if g == nil {
		return nil, ErrUnverified
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	entry, err := g.restartEntryLocked(ctx)
	if err != nil {
		return nil, err
	}
	dir, cmd, err := restartDirection(entry)
	if err != nil {
		return nil, err
	}
	return &RestartAuthority{minted: true, guard: g, binding: g.binding, direction: dir, command: cmd, state: entry.State}, nil
}

// Check re-proves, under the still-held lock, that the authority is the same
// one the lifecycle justifies right now. Call it before every effect.
func (a *RestartAuthority) Check(ctx context.Context) error {
	if a == nil || !a.minted || a.guard == nil {
		return ErrUnverified
	}
	g := a.guard
	g.mu.Lock()
	defer g.mu.Unlock()
	return a.checkLocked(ctx)
}

// WithLockedReconciliation runs one remove-only reconciliation effect while
// retaining the guard mutex. Authority is re-proved before and after the
// effect, so Close and lifecycle supersession cannot release or change the
// held authority between its last check and a destructive host operation.
// The effect must not call guard or authority methods: the mutex is not
// reentrant. This does not mint a live Lease or authorize forward Apply.
func (a *RestartAuthority) WithLockedReconciliation(ctx context.Context, effect func() error) error {
	if a == nil || !a.minted || a.guard == nil || effect == nil {
		return ErrUnverified
	}
	g := a.guard
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := a.checkLocked(ctx); err != nil {
		return err
	}
	err := effect()
	return errors.Join(err, a.checkLocked(ctx))
}

// checkLocked requires the caller to hold a.guard.mu.
func (a *RestartAuthority) checkLocked(ctx context.Context) error {
	g := a.guard
	if g.binding != a.binding {
		return ErrUnverified
	}
	entry, err := g.restartEntryLocked(ctx)
	if err != nil {
		return err
	}
	dir, cmd, err := restartDirection(entry)
	if err != nil {
		return err
	}
	if dir != a.direction || !sameCommandIntent(cmd, a.command) {
		return ErrUnverified
	}
	return nil
}

// RecordRestartReconciled durably leaves the operation in
// NEEDS_RECONCILIATION after the host effects were finished. It changes the
// state field only (never Leased, Commands or Superseded) and never releases
// the port reservation: the numeric port stays fenced until a separately
// verified release exists, exactly as after an in-process Retire.
func (g *PendingGuard) RecordRestartReconciled(ctx context.Context, a *RestartAuthority) error {
	if g == nil || a == nil || a.guard != g {
		return ErrUnverified
	}
	if err := a.Check(ctx); err != nil {
		return errors.Join(ErrUnverified, err)
	}
	return g.RecordReconciliationNeeded(ctx, a.command)
}
