package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressownership"
	"github.com/zarkmakerburg/baft/internal/release"
)

// Only RunOnce can supply the original verified envelope here. The generic
// executeFull entrypoint still refuses operational v1 and the legacy injected
// LAB executor is never consulted. All host authority and executor bootstrap
// inputs come from independently protected, fixed local paths.
func (a *Agent) executeSignedOperationalIngress(ctx context.Context, envelope release.Envelope, job agentjob.Job) (string, string, error) {
	a.operationalIngressMu.Lock()
	defer a.operationalIngressMu.Unlock()
	command, err := ingressV1Command(job)
	if err != nil {
		return "", "", err
	}
	blocked := func(cause error) (string, string, error) {
		return "operational ingress remains blocked", "", errors.Join(cause, a.publicIngressOperationalRefusal(ctx, job))
	}
	// Construct once per process: a restarted Agent must not adopt a persisted
	// journal as a live listener. New Apply/Retire share this actual runtime.
	if a.operationalIngressExecutor == nil {
		x, err := ingressownership.NewInstalledManagedLABExecutor(ctx, a.cfg.NodeID)
		if err != nil {
			return blocked(err)
		}
		a.operationalIngressExecutor = x
	}
	store, err := ingressauthority.OpenInstalledManagedReservationStore(ctx, a.cfg.NodeID)
	if err != nil {
		return blocked(err)
	}
	if err := store.ReservePending(command.Binding); err != nil {
		return blocked(err)
	}
	guard, err := store.HoldPending(command.Binding)
	if err != nil {
		return blocked(err)
	}
	defer guard.Close()
	if _, err := guard.BindInstalledPolicy(ctx); err != nil {
		return blocked(err)
	}
	if err := guard.MarkApplying(ctx, command.Binding); err != nil {
		return blocked(err)
	}
	if err := guard.RecordCommandIntent(ctx, command); err != nil {
		return blocked(err)
	}
	lease, err := a.MintSignedHeldIngressLease(ctx, envelope, command, guard)
	if err != nil {
		return blocked(err)
	}
	defer lease.Close()
	receipt := ingressauthority.ReceiptV1{SchemaVersion: 1, JobID: command.JobID, Action: command.Action, Binding: command.Binding, OwnershipVerified: true, RollbackStatus: "not_needed"}
	x := a.operationalIngressExecutor
	if command.Action == agentjob.ActionPublicIngressApply {
		observed, err := x.Apply(ctx, command, lease)
		if err != nil {
			return "", "", errors.Join(err, guard.RecordReconciliationNeeded(ctx, command))
		}
		receipt.Phase = "applied"
		receipt.ListenerEvidence = observed.Listener
		receipt.FirewallEvidence = []ingressauthority.FirewallEvidenceV1{observed.Firewall}
	} else {
		observed, err := x.Retire(ctx, command, lease)
		if err != nil {
			return "", "", errors.Join(err, guard.RecordReconciliationNeeded(ctx, command))
		}
		receipt.Phase = "retired"
		receipt.FirewallEvidence = []ingressauthority.FirewallEvidenceV1{observed.Firewall}
	}
	if err := lease.RecordInertOutcome(ctx, receipt); err != nil {
		return "", "", errors.Join(err, guard.RecordReconciliationNeeded(ctx, command))
	}
	if err := a.postVerifiedIngressReceipt(ctx, receipt, command, lease); err != nil {
		return "", "", errors.Join(err, guard.RecordReconciliationNeeded(ctx, command))
	}
	return fmt.Sprintf("operational ingress %s with authenticated receipt", receipt.Phase), "", nil
}
