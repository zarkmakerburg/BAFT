package ingressownership

import (
	"context"
	"errors"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressinventory"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// NFTLabTransaction joins the isolated runtime, a preprovisioned nft chain,
// and a durable journal. It is not a signed Agent executor. Restart always
// requires independent socket and kernel reconciliation before any adoption.
type NFTLabTransaction struct {
	mu       sync.Mutex
	Runtime  *Runtime
	Firewall *NFTLabBackend
	Journal  MutationJournal
	// Successful socket retirement is process-local evidence, never recovered
	// from journal phases. Keep it only while exact-owner cleanup is unfinished.
	stopped map[ingressmanifest.Proof]stoppedIngressSocket
}

type stoppedIngressSocket struct {
	runtime     *Runtime
	firewall    *NFTLabBackend
	handle      uint64
	chainHandle uint64
	removed     bool
	rollback    bool
}

func nftTransactionRecord(spec ingressmanifest.Spec, proof ingressmanifest.Proof) (MutationRecord, RulePlan, error) {
	expected, err := ingressmanifest.Preview(spec)
	if err != nil || expected != proof {
		return MutationRecord{}, RulePlan{}, ErrInvalid
	}
	_, portText, err := net.SplitHostPort(spec.Config.Listen)
	if err != nil {
		return MutationRecord{}, RulePlan{}, ErrInvalid
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return MutationRecord{}, RulePlan{}, ErrInvalid
	}
	plan, err := PlanIngressRule(uint16(port), proof)
	return MutationRecord{Port: uint16(port), Owner: proof, Phase: PhaseIntent}, plan, err
}

// effectLeaseFence is private to this implementation. Callers cannot supply
// a test double, Boolean or authorization callback: the ONLY positive guard
// is Alpha's concrete held *Lease, rechecked before every effect boundary.
type effectLeaseFence struct {
	lease   *ingressauthority.Lease
	command ingressauthority.CommandV1
	// Private per-operation observation: never caller-supplied and never a permit.
	pin         ingressinventory.ManagedLABExecutionPin
	transaction *NFTLabTransaction
}

func (f *effectLeaseFence) check(ctx context.Context) error {
	if f == nil {
		// Legacy LAB rollback may use a fresh cleanup context after
		// the original Apply context is cancelled. Preserve existing
		// LAB cancellation/rollback behavior; only live operational
		// leases get the additional per-effect authority fence.
		return nil
	}
	if err := CheckIngressExecutorAuthority(ctx, f.lease, f.command); err != nil {
		return err
	}
	return f.checkScope(ctx)
}

// checkScope is called under Alpha's guard mutex by run; it MUST NOT call
// lease/guard methods recursively.
func (f *effectLeaseFence) checkScope(ctx context.Context) error {
	// Reobserve installed profile, exact parsed source file membership/bytes,
	// current executable, cgroup and namespace BEFORE each owned mutation.
	// On drift, do not continue effects or attempt unleased cleanup.
	if err := ingressinventory.CheckInstalledManagedLABAtEffect(ctx, f.command.Binding.NodeID, f.pin); err != nil {
		return err
	}
	if f.transaction == nil {
		return ErrExecutorTrustUnavailable
	}
	return f.transaction.checkOperationalEffectPort(ctx, f.command.Binding)
}

// run serializes the concrete OS-held guard against Close throughout each
// leaf mutation. A read-only check followed by an unlocked mutation is not
// sufficient when another goroutine closes the lease or held guard.
func (f *effectLeaseFence) run(ctx context.Context, effect func() error) error {
	if f == nil {
		return effect()
	}
	return f.lease.WithLockedIngress(ctx, f.command, func() error {
		if err := f.checkScope(ctx); err != nil {
			return err
		}
		return effect()
	})
}

func (t *NFTLabTransaction) ApplyWithLease(ctx context.Context, spec ingressmanifest.Spec, proof ingressmanifest.Proof, cmd ingressauthority.CommandV1, lease *ingressauthority.Lease) error {
	if lease == nil || cmd.Action != "public_ingress_apply" || cmd.Binding.Spec != spec || cmd.Binding.Proof != proof {
		return ErrExecutorTrustUnavailable
	}
	// Refuse unsigned/unheld operations before even capturing private scope.
	if err := CheckIngressExecutorAuthority(ctx, lease, cmd); err != nil {
		return err
	}
	pin, err := ingressinventory.PinInstalledManagedLABExecution(ctx, cmd.Binding.NodeID)
	if err != nil {
		return err
	}
	fence := &effectLeaseFence{lease: lease, command: cmd, pin: pin, transaction: t}
	if err := fence.check(ctx); err != nil {
		return err
	}
	return t.apply(ctx, spec, proof, fence)
}
func (t *NFTLabTransaction) Apply(ctx context.Context, spec ingressmanifest.Spec, proof ingressmanifest.Proof) error {
	return t.apply(ctx, spec, proof, nil)
}
func (t *NFTLabTransaction) apply(ctx context.Context, spec ingressmanifest.Spec, proof ingressmanifest.Proof, fence *effectLeaseFence) error {
	if t == nil || ctx == nil || t.Runtime == nil || t.Firewall == nil || t.Journal == nil {
		return ErrInvalid
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := fence.check(ctx); err != nil {
		return err
	}
	record, plan, err := nftTransactionRecord(spec, proof)
	if err != nil {
		return err
	}
	endpoint, err := canonicalEndpoint(spec.Config.Listen)
	if err != nil {
		return err
	}
	current, found, err := t.Journal.Current(proof)
	if err != nil {
		return err
	}
	t.Runtime.mu.Lock()
	existing := t.Runtime.sessions[endpoint]
	t.Runtime.mu.Unlock()
	if existing != nil {
		if !found || current.Owner != proof || current.Port != plan.Port || current.Phase != PhaseVerified || current.FirewallHandle == 0 || existing.proof != proof {
			return ErrUnverified
		}
		observed, err := t.Firewall.Inspect(ctx, plan)
		if err != nil || observed.Handle != current.FirewallHandle {
			return errors.Join(ErrFirewallUnverified, err)
		}
		if err := fence.check(ctx); err != nil {
			return err
		}
		return fence.run(ctx, func() error { return t.Runtime.Apply(ctx, spec, proof) })
	}
	if found && (current.Owner != proof || current.Port != plan.Port || current.Phase != PhaseIntent) {
		return ErrUnverified
	}
	if err := fence.check(ctx); err != nil {
		return err
	}
	if err := fence.run(ctx, func() error { return t.Journal.Advance(record) }); err != nil {
		return err
	}
	if err := fence.check(ctx); err != nil {
		return err
	}
	if err := fence.run(ctx, func() error { return t.Runtime.Apply(ctx, spec, proof) }); err != nil {
		return err
	}
	record.Phase = PhaseListenerBound
	if err := fence.check(ctx); err != nil {
		return err
	}
	if err := fence.run(ctx, func() error { return t.Journal.Advance(record) }); err != nil {
		if fence != nil {
			// Operational lease might have disappeared while recording.
			// Do not close even our own listener without first checking
			// the original held lock under a fresh valid operation context.
			// Leave the pending claimed state for Alpha reconciliation.
			if leaseErr := fence.check(ctx); leaseErr != nil {
				return errors.Join(err, leaseErr)
			}
		}
		return errors.Join(err, fence.run(ctx, func() error { return t.Runtime.Retire(endpoint, proof) }))
	}
	if err := ctx.Err(); err != nil {
		if fence != nil {
			return err
		}
		return errors.Join(err, fence.run(ctx, func() error { return t.Runtime.Retire(endpoint, proof) }))
	}
	if err := fence.check(ctx); err != nil {
		// Do not roll back without the held lock. Durable recovery retains
		// the exact in-progress operation as uncertain.
		return err
	}
	var observed FirewallObservation
	err = fence.run(ctx, func() error {
		var installErr error
		observed, installErr = t.Firewall.Install(ctx, plan)
		return installErr
	})
	rollback := func(cause error) error {
		if err := fence.check(ctx); err != nil {
			return errors.Join(cause, err)
		}
		stopErr := fence.run(ctx, func() error { return t.Runtime.Retire(endpoint, proof) })
		if !listenerRetirementVerified(stopErr) {
			return errors.Join(cause, stopErr)
		}
		if observed.Handle == 0 {
			return errors.Join(cause, stopErr)
		}
		if t.stopped == nil {
			t.stopped = make(map[ingressmanifest.Proof]stoppedIngressSocket)
		}
		t.stopped[proof] = stoppedIngressSocket{runtime: t.Runtime, firewall: t.Firewall, handle: observed.Handle, chainHandle: t.Firewall.ExpectedChainHandle, rollback: true}
		// Cleanup must outlive a cancelled apply, but must not block the
		// transaction lock indefinitely if the kernel backend is unresponsive.
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return errors.Join(cause, stopErr, t.retireLocked(cleanupCtx, spec, proof, fence))
	}
	if err != nil {
		return rollback(err)
	}
	record.FirewallHandle = observed.Handle
	record.Phase = PhaseFirewallInstalled
	if err := fence.check(ctx); err != nil {
		return err
	}
	if err := fence.run(ctx, func() error { return t.Journal.Advance(record) }); err != nil {
		return rollback(err)
	}
	if err := ctx.Err(); err != nil {
		return rollback(err)
	}
	if err := fence.check(ctx); err != nil {
		return err
	}
	if latest, err := t.Firewall.Inspect(ctx, plan); err != nil || latest.Handle != observed.Handle {
		return rollback(errors.Join(ErrFirewallUnverified, err))
	}
	record.Phase = PhaseVerified
	if err := fence.check(ctx); err != nil {
		return err
	}
	if err := fence.run(ctx, func() error { return t.Journal.Advance(record) }); err != nil {
		return rollback(err)
	}
	if err := ctx.Err(); err != nil {
		return rollback(err)
	}
	if err := fence.check(ctx); err != nil {
		return err
	}
	return nil
}

func (t *NFTLabTransaction) RetireWithLease(ctx context.Context, spec ingressmanifest.Spec, proof ingressmanifest.Proof, cmd ingressauthority.CommandV1, lease *ingressauthority.Lease) error {
	if lease == nil || cmd.Action != "public_ingress_retire" || cmd.Binding.Spec != spec || cmd.Binding.Proof != proof {
		return ErrExecutorTrustUnavailable
	}
	// Refuse unsigned/unheld operations before even capturing private scope.
	if err := CheckIngressExecutorAuthority(ctx, lease, cmd); err != nil {
		return err
	}
	pin, err := ingressinventory.PinInstalledManagedLABExecution(ctx, cmd.Binding.NodeID)
	if err != nil {
		return err
	}
	fence := &effectLeaseFence{lease: lease, command: cmd, pin: pin, transaction: t}
	if err := fence.check(ctx); err != nil {
		return err
	}
	return t.retire(ctx, spec, proof, fence)
}
func (t *NFTLabTransaction) Retire(ctx context.Context, spec ingressmanifest.Spec, proof ingressmanifest.Proof) error {
	return t.retire(ctx, spec, proof, nil)
}
func (t *NFTLabTransaction) retire(ctx context.Context, spec ingressmanifest.Spec, proof ingressmanifest.Proof, fence *effectLeaseFence) error {
	if t == nil || ctx == nil || t.Runtime == nil || t.Firewall == nil || t.Journal == nil {
		return ErrInvalid
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.retireLocked(ctx, spec, proof, fence)
}

// retireLocked shares exact-owner cleanup with failed Apply while t.mu is held.
func (t *NFTLabTransaction) retireLocked(ctx context.Context, spec ingressmanifest.Spec, proof ingressmanifest.Proof, fence *effectLeaseFence) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := fence.check(ctx); err != nil {
		return err
	}
	record, plan, err := nftTransactionRecord(spec, proof)
	if err != nil {
		return err
	}
	current, found, err := t.Journal.Current(proof)
	if err != nil {
		return err
	}
	stopped, alreadyStopped := t.stopped[proof]
	if !found || current.Owner != proof || current.Port != plan.Port {
		return ErrUnverified
	}
	if alreadyStopped && (stopped.runtime != t.Runtime || stopped.firewall != t.Firewall || stopped.chainHandle != t.Firewall.ExpectedChainHandle) {
		return ErrUnverified
	}
	if alreadyStopped && stopped.rollback && (current.Phase == PhaseListenerBound || current.Phase == PhaseFirewallInstalled) {
		if stopped.handle == 0 || (current.Phase == PhaseListenerBound && current.FirewallHandle != 0) || (current.Phase == PhaseFirewallInstalled && current.FirewallHandle != stopped.handle) {
			return ErrUnverified
		}
		observed, err := t.Firewall.Inspect(ctx, plan)
		if err != nil || observed.Handle != stopped.handle {
			return errors.Join(ErrFirewallUnverified, err)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		record.Phase, record.FirewallHandle = PhaseRetireIntent, stopped.handle
		if err := fence.check(ctx); err != nil {
			return err
		}
		if err := fence.run(ctx, func() error { return t.Journal.Advance(record) }); err != nil {
			return err
		}
		current = record
	}
	if current.FirewallHandle == 0 || (current.Phase != PhaseVerified && current.Phase != PhaseRetireIntent && !(current.Phase == PhaseRetired && alreadyStopped && stopped.removed)) || (alreadyStopped && stopped.handle != current.FirewallHandle) {
		return ErrUnverified
	}
	if alreadyStopped && stopped.removed {
		if current.Phase != PhaseRetireIntent && current.Phase != PhaseRetired {
			return ErrUnverified
		}
		if err := fence.check(ctx); err != nil {
			return err
		}
		if err := t.Firewall.verifyAbsent(ctx, plan, stopped.handle); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		record.Phase, record.FirewallHandle = PhaseRetired, current.FirewallHandle
		if err := fence.check(ctx); err != nil {
			return err
		}
		if err := fence.run(ctx, func() error { return t.Journal.Advance(record) }); err != nil {
			return err
		}
		delete(t.stopped, proof)
		return nil
	}
	observed, err := t.Firewall.Inspect(ctx, plan)
	if err != nil {
		return err
	}
	if observed.Handle != current.FirewallHandle {
		return ErrFirewallUnverified
	}
	record.Phase = PhaseRetireIntent
	record.FirewallHandle = current.FirewallHandle
	if err := fence.check(ctx); err != nil {
		return err
	}
	if err := fence.run(ctx, func() error { return t.Journal.Advance(record) }); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// A terminal transport error must not mask *successful exact socket
	// retirement*. Only a typed verified retirement permits deleting the
	// independently inspected exact-owned nft rule; preserve that cause in
	// the outcome even when firewall/journal cleanup completes.
	var retireErr error
	if !alreadyStopped {
		if err := fence.check(ctx); err != nil {
			return err
		}
		retireErr = fence.run(ctx, func() error { return t.Runtime.Retire(spec.Config.Listen, proof) })
		if !listenerRetirementVerified(retireErr) {
			return retireErr
		}
		if t.stopped == nil {
			t.stopped = make(map[ingressmanifest.Proof]stoppedIngressSocket)
		}
		stopped = stoppedIngressSocket{runtime: t.Runtime, firewall: t.Firewall, handle: current.FirewallHandle, chainHandle: t.Firewall.ExpectedChainHandle}
		t.stopped[proof] = stopped
	}
	if err := fence.check(ctx); err != nil {
		return errors.Join(retireErr, err)
	}
	if err := fence.run(ctx, func() error { return t.Firewall.Remove(ctx, plan, observed.Handle) }); err != nil {
		return errors.Join(retireErr, err)
	}
	stopped.removed = true
	t.stopped[proof] = stopped
	record.Phase = PhaseRetired
	if err := fence.check(ctx); err != nil {
		return err
	}
	if err := fence.run(ctx, func() error { return t.Journal.Advance(record) }); err != nil {
		return errors.Join(retireErr, err)
	}
	delete(t.stopped, proof)
	return retireErr
}
