package ingressownership

import (
	"context"
	"errors"
	"net"
	"strconv"
	"sync"
	"time"

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

func (t *NFTLabTransaction) Apply(ctx context.Context, spec ingressmanifest.Spec, proof ingressmanifest.Proof) error {
	if t == nil || ctx == nil || t.Runtime == nil || t.Firewall == nil || t.Journal == nil {
		return ErrInvalid
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := ctx.Err(); err != nil {
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
		return t.Runtime.Apply(ctx, spec, proof)
	}
	if found && (current.Owner != proof || current.Port != plan.Port || current.Phase != PhaseIntent) {
		return ErrUnverified
	}
	if err := t.Journal.Advance(record); err != nil {
		return err
	}
	if err := t.Runtime.Apply(ctx, spec, proof); err != nil {
		return err
	}
	record.Phase = PhaseListenerBound
	if err := t.Journal.Advance(record); err != nil {
		return errors.Join(err, t.Runtime.Retire(endpoint, proof))
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(err, t.Runtime.Retire(endpoint, proof))
	}
	observed, err := t.Firewall.Install(ctx, plan)
	rollback := func(cause error) error {
		stopErr := t.Runtime.Retire(endpoint, proof)
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
		return errors.Join(cause, stopErr, t.retireLocked(cleanupCtx, spec, proof))
	}
	if err != nil {
		return rollback(err)
	}
	record.FirewallHandle = observed.Handle
	record.Phase = PhaseFirewallInstalled
	if err := t.Journal.Advance(record); err != nil {
		return rollback(err)
	}
	if err := ctx.Err(); err != nil {
		return rollback(err)
	}
	if latest, err := t.Firewall.Inspect(ctx, plan); err != nil || latest.Handle != observed.Handle {
		return rollback(errors.Join(ErrFirewallUnverified, err))
	}
	record.Phase = PhaseVerified
	if err := t.Journal.Advance(record); err != nil {
		return rollback(err)
	}
	if err := ctx.Err(); err != nil {
		return rollback(err)
	}
	return nil
}

func (t *NFTLabTransaction) Retire(ctx context.Context, spec ingressmanifest.Spec, proof ingressmanifest.Proof) error {
	if t == nil || ctx == nil || t.Runtime == nil || t.Firewall == nil || t.Journal == nil {
		return ErrInvalid
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.retireLocked(ctx, spec, proof)
}

// retireLocked shares exact-owner cleanup with failed Apply while t.mu is held.
func (t *NFTLabTransaction) retireLocked(ctx context.Context, spec ingressmanifest.Spec, proof ingressmanifest.Proof) error {
	if err := ctx.Err(); err != nil {
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
		if err := t.Journal.Advance(record); err != nil {
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
		if err := t.Firewall.verifyAbsent(ctx, plan, stopped.handle); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		record.Phase, record.FirewallHandle = PhaseRetired, current.FirewallHandle
		if err := t.Journal.Advance(record); err != nil {
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
	if err := t.Journal.Advance(record); err != nil {
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
		retireErr = t.Runtime.Retire(spec.Config.Listen, proof)
		if !listenerRetirementVerified(retireErr) {
			return retireErr
		}
		if t.stopped == nil {
			t.stopped = make(map[ingressmanifest.Proof]stoppedIngressSocket)
		}
		stopped = stoppedIngressSocket{runtime: t.Runtime, firewall: t.Firewall, handle: current.FirewallHandle, chainHandle: t.Firewall.ExpectedChainHandle}
		t.stopped[proof] = stopped
	}
	if err := t.Firewall.Remove(ctx, plan, observed.Handle); err != nil {
		return errors.Join(retireErr, err)
	}
	stopped.removed = true
	t.stopped[proof] = stopped
	record.Phase = PhaseRetired
	if err := t.Journal.Advance(record); err != nil {
		return errors.Join(retireErr, err)
	}
	delete(t.stopped, proof)
	return retireErr
}
