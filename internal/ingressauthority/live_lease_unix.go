//go:build unix

package ingressauthority

import (
	"context"
	"crypto/ed25519"
	"errors"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressinventory"
	"github.com/zarkmakerburg/baft/internal/release"
)

// The private state is never supplied by a caller or reconstructed from JSON.
// In particular, a durable minted marker is a replay fence, not a permit.
type liveLeaseState struct {
	guard   *PendingGuard
	command CommandV1
	pin     ingressinventory.ManagedLABExecutionPin
	key     enrolledJobKeyIdentity
	closed  bool // serialized by guard.mu
}

// MintSignedManagedLABLease is restricted to the independently measured
// singleton, loopback-only Linux LAB. It verifies the original BCC envelope
// against the fixed root-protected enrollment key, binds the canonical signed
// command to the held durable intent and privately obtains its execution pin.
// Neither a caller key, a caller pin, a boolean nor unsigned JSON can mint it.
// The guard must remain held until all effects and outcome persistence finish.
// Reopening state after a crash cannot mint a second lease for this JobID.
func (g *PendingGuard) MintSignedManagedLABLease(ctx context.Context, env release.Envelope) (*Lease, error) {
	if g == nil || ctx == nil || ctx.Err() != nil {
		return nil, ErrUnverified
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.checkPolicyPendingLocked(ctx); err != nil {
		return nil, err
	}
	if err := checkInstalledAuthorityStore(ctx, g.store); err != nil {
		return nil, err
	}
	key, err := loadEnrolledJobKey(ctx)
	if err != nil {
		return nil, err
	}
	job, err := (agentjob.Verifier{BCCKey: ed25519.PublicKey(key.Public[:]), NodeID: g.binding.NodeID}).Verify(env, time.Now())
	if err != nil {
		return nil, err
	}
	command, err := CommandFromJob(job)
	if err != nil || (command.Action != commandIntentApply && command.Action != commandIntentRetire) {
		return nil, ErrBinding
	}
	if err := g.checkHeldCommandLocked(ctx, command); err != nil {
		return nil, err
	}
	pin, err := ingressinventory.PinInstalledManagedLABExecution(ctx, g.binding.NodeID)
	if err != nil {
		return nil, err
	}
	entries, err := readLifecycle(g.root)
	if err != nil {
		return nil, err
	}
	entry := entries[reservationKey(g.binding)]
	if entry.MintedJobs[command.JobID] {
		return nil, ErrUnverified
	}
	if entry.MintedJobs == nil {
		entry.MintedJobs = make(map[string]bool)
	}
	entry.MintedJobs[command.JobID] = true
	entries[reservationKey(g.binding)] = entry
	// Crash between this fsynced write and return leaves a reserved,
	// uncertain operation. It never enables recovered execution.
	if err := writeLifecycle(g.fd, g.root, entries); err != nil {
		return nil, err
	}
	l := &LiveLease{minted: true, binding: g.binding, state: &liveLeaseState{guard: g, command: command, pin: pin, key: key}}
	if err := checkLiveLeaseLocked(l, ctx, command); err != nil {
		return nil, err
	}
	return l, nil
}

func checkLiveLease(l *LiveLease, ctx context.Context, command CommandV1) error {
	if l.state == nil || l.state.guard == nil {
		return ErrUnverified
	}
	g := l.state.guard
	g.mu.Lock()
	defer g.mu.Unlock()
	return checkLiveLeaseLocked(l, ctx, command)
}

func checkLiveLeaseLocked(l *LiveLease, ctx context.Context, command CommandV1) error {
	s := l.state
	if s == nil || s.guard == nil || s.closed || !sameCommandIntent(s.command, command) {
		return ErrUnverified
	}
	if err := s.guard.checkHeldCommandLocked(ctx, command); err != nil {
		return err
	}
	if err := checkInstalledAuthorityStore(ctx, s.guard.store); err != nil {
		return err
	}
	entries, err := readLifecycle(s.guard.root)
	if err != nil || !entries[reservationKey(l.binding)].MintedJobs[command.JobID] {
		return ErrUnverified
	}
	key, err := loadEnrolledJobKey(ctx)
	if err != nil || key != s.key {
		return ErrUnverified
	}
	if err := ingressinventory.CheckInstalledManagedLABAtEffect(ctx, l.binding.NodeID, s.pin); err != nil {
		return err
	}
	return s.guard.checkHeldCommandLocked(ctx, command)
}

// Close revokes this same-process capability, without releasing its durable
// reservation or closing the caller-owned PendingGuard. Copying a Lease does
// not duplicate state: every copy shares this revocation and original guard.
func (l *LiveLease) Close() error {
	if l == nil || l.state == nil || l.state.guard == nil {
		return ErrUnverified
	}
	l.state.guard.mu.Lock()
	defer l.state.guard.mu.Unlock()
	if l.state.closed {
		return ErrUnverified
	}
	l.state.closed = true
	return nil
}

// WithLockedIngress serializes one leaf host effect with lease/guard closure.
// The effect is not an authority callback: it executes only after the same
// original private live state is revalidated, with the OS guard still held.
// The callback must not call guard/lease methods (the mutex is not reentrant).
// Incomplete effects keep their original error and the durable reservation.
func (l *LiveLease) WithLockedIngress(ctx context.Context, command CommandV1, effect func() error) error {
	if l == nil || !l.minted || l.binding != command.Binding || l.state == nil || l.state.guard == nil || effect == nil {
		return ErrUnverified
	}
	l.state.guard.mu.Lock()
	defer l.state.guard.mu.Unlock()
	if err := checkLiveLeaseLocked(l, ctx, command); err != nil {
		return err
	}
	err := effect()
	return errors.Join(err, checkLiveLeaseLocked(l, ctx, command))
}
