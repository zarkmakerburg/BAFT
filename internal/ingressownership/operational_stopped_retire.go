package ingressownership

import (
	"context"
	"errors"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressinventory"
)

// stoppedRetireOwner permits only same-process recovery classification.
// Journal phases and missing runtime maps are never sufficient: successful
// exact socket retirement must have been retained privately by this very
// transaction, with unchanged runtime, backend and pinned chain/rule handle.
// The normal entrypoint additionally requires the same concrete held Lease.
func (x *OperationalExecutor) stoppedRetireOwner(ctx context.Context, b ingressauthority.BindingV1, observed ingressinventory.CoverageEvidence) error {
	t := x.transaction
	t.mu.Lock()
	defer t.mu.Unlock()
	record, found, err := t.Journal.Current(b.Proof)
	if err != nil || !found || record.Owner != b.Proof || record.Port != b.Port ||
		record.FirewallHandle == 0 || (record.Phase != PhaseRetireIntent && record.Phase != PhaseRetired) {
		return errors.Join(ErrUnverified, err)
	}
	stopped, ok := t.stopped[b.Proof]
	if !ok || stopped.runtime != t.Runtime || stopped.firewall != t.Firewall ||
		stopped.chainHandle != t.Firewall.ExpectedChainHandle ||
		stopped.handle != record.FirewallHandle || (record.Phase == PhaseRetired && !stopped.removed) {
		return ErrUnverified
	}
	if err := retiredPortObservation(b, observed); err != nil {
		return err
	}
	// Runtime state is checked as an additional veto, never as proof of
	// kernel cleanup. The protected collector above must observe no listener.
	t.Runtime.mu.Lock()
	_, present := t.Runtime.sessions[b.ListenEndpoint]
	t.Runtime.mu.Unlock()
	if present {
		return ErrUnverified
	}
	plan, err := PlanIngressRule(b.Port, b.Proof)
	if err != nil {
		return err
	}
	if stopped.removed {
		if err := t.Firewall.verifyAbsent(ctx, plan, stopped.handle); err != nil {
			return err
		}
	} else {
		rule, err := t.Firewall.Inspect(ctx, plan)
		if err != nil || rule.Handle != stopped.handle {
			return errors.Join(ErrFirewallUnverified, err)
		}
	}
	return ctx.Err()
}

// A fresh protected collector observation is required before retry and after
// final cleanup. A foreign listener on any alias/family or stopped reservation
// prevents declaring the numerical port retired. TIME_WAIT is not a listener.
func retiredPortObservation(b ingressauthority.BindingV1, observed ingressinventory.CoverageEvidence) error {
	if observed.Port != b.Port || !observed.CoverageComplete || len(observed.UnknownSources) != 0 {
		return ingressinventory.ErrCoverageIncomplete
	}
	for _, claim := range observed.Stopped {
		if claim.Port == b.Port {
			return ErrConflict
		}
	}
	for _, entry := range observed.ActiveTCP {
		if entry.Port != b.Port {
			return ErrUnverified
		}
		if entry.State == "0A" {
			return ErrConflict
		}
	}
	return nil
}
