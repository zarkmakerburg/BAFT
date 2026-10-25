package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressinventory"
	"github.com/zarkmakerburg/baft/internal/ingressownership"
	"github.com/zarkmakerburg/baft/internal/release"
)

// IngressOperationalExecutor is satisfied by
// *ingressownership.OperationalExecutor. It can do nothing without a live
// *ingressauthority.Lease, which only MintIngressLease can produce.
type IngressOperationalExecutor interface {
	Apply(context.Context, ingressauthority.CommandV1, *ingressauthority.Lease) (ingressownership.ObservedIngressV1, error)
	Retire(context.Context, ingressauthority.CommandV1, *ingressauthority.Lease) (ingressownership.RetiredIngressEvidence, error)
}

// IngressOperational enables the real signed v1 public-ingress path. A nil
// value (the default) keeps every v1 job release-closed: a BLOCKED receipt and
// a failed ACK. It never selects the legacy LAB executor.
type IngressOperational struct {
	Store    *ingressauthority.ReservationStore
	Executor IngressOperationalExecutor
}

const ingressCleanupTimeout = 30 * time.Second

// publicIngressOperational runs one signed v1 Apply/Retire through the held
// authority chain. Everything before MarkApplying is effect-free and reports a
// BLOCKED receipt. After MarkApplying (Apply) or lease mint (Retire) any error,
// cancellation or panic records NEEDS_RECONCILIATION; cleanup is never inferred.
func (a *Agent) publicIngressOperational(ctx context.Context, j agentjob.Job, env *release.Envelope) (detail string, err error) {
	op := a.cfg.IngressOperational
	if op == nil || op.Store == nil || op.Executor == nil || env == nil {
		return "operational ingress remains blocked", a.publicIngressOperationalRefusal(ctx, j)
	}
	if ctx == nil || a.cfg.NodeID != j.NodeID {
		return "", ingressauthority.ErrUnverified
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	now := a.cfg.Now
	if now == nil {
		now = time.Now
	}
	// Admission: the sealed evidence must come from the exact envelope that
	// carried this job; no Seen callback here because RunOnce already claimed
	// the job ID durably, and the lease mint is separately single-use.
	verified, verr := (agentjob.Verifier{BCCKey: a.cfg.BCCJobKey, NodeID: a.cfg.NodeID}).VerifyAdmitted(*env, now())
	if verr != nil {
		return "", verr
	}
	signed := verified.Job()
	if signed.JobID != j.JobID || signed.NodeID != j.NodeID || signed.Action != j.Action {
		return "", ingressauthority.ErrBinding
	}
	command, cerr := ingressV1Command(signed)
	if cerr != nil {
		return "", cerr
	}
	if cerr := command.Validate(now()); cerr != nil {
		return "", cerr
	}
	blocked := func(cause error) (string, error) {
		return "operational ingress blocked", errors.Join(cause, a.postOperationalReceipt(ctx, command, blockedReceipt(command, cause)))
	}

	if command.Action == agentjob.ActionPublicIngressApply {
		if rerr := op.Store.ReservePending(command.Binding); rerr != nil {
			return blocked(rerr)
		}
	}
	guard, herr := op.Store.HoldPending(command.Binding)
	if herr != nil {
		return blocked(herr)
	}
	defer guard.Close()
	if _, perr := guard.BindInstalledPolicy(ctx); perr != nil {
		return blocked(fmt.Errorf("trusted host policy unavailable: %w", perr))
	}
	if ierr := guard.RecordCommandIntent(ctx, command); ierr != nil {
		return blocked(ierr)
	}

	effects := false
	defer func() {
		if r := recover(); r != nil {
			if effects {
				_ = a.recordReconciliation(ctx, guard, command)
			}
			panic(r)
		}
	}()
	if command.Action == agentjob.ActionPublicIngressApply {
		// Durably record possible partial effects BEFORE any executor call.
		effects = true
		if merr := guard.MarkApplying(ctx, command.Binding); merr != nil {
			return a.failedAfterEffects(ctx, guard, command, merr)
		}
	}
	lease, lerr := a.MintIngressLease(ctx, *env, command, guard)
	if lerr != nil {
		if effects {
			return a.failedAfterEffects(ctx, guard, command, lerr)
		}
		return blocked(lerr)
	}
	effects = true

	switch command.Action {
	case agentjob.ActionPublicIngressApply:
		observed, xerr := op.Executor.Apply(ctx, command, lease)
		if xerr != nil {
			return a.executorFailure(ctx, guard, command, xerr)
		}
		if verr := ingressownership.ValidateObservedIngressV1(command.Binding, observed); verr != nil {
			return a.failedAfterEffects(ctx, guard, command, verr)
		}
		receipt := ingressauthority.ReceiptV1{
			SchemaVersion: 1, JobID: command.JobID, Action: command.Action, Binding: command.Binding,
			Phase: "applied", ListenerEvidence: observed.Listener,
			FirewallEvidence: []ingressauthority.FirewallEvidenceV1{observed.Firewall}, OwnershipVerified: true,
		}
		if perr := a.postOperationalReceipt(ctx, command, receipt); perr != nil {
			return a.failedAfterEffects(ctx, guard, command, fmt.Errorf("applied receipt not delivered: %w", perr))
		}
		return "operational ingress applied; exact ownership verified", nil
	default:
		evidence, xerr := op.Executor.Retire(ctx, command, lease)
		if xerr != nil {
			return a.executorFailure(ctx, guard, command, xerr)
		}
		if evidence.Port != command.Binding.Port || evidence.Owner != command.Binding.Proof || evidence.Phase != ingressownership.PhaseRetired {
			return a.failedAfterEffects(ctx, guard, command, errors.New("retire evidence does not match command"))
		}
		receipt := ingressauthority.ReceiptV1{
			SchemaVersion: 1, JobID: command.JobID, Action: command.Action, Binding: command.Binding, Phase: "retired",
		}
		if perr := a.postOperationalReceipt(ctx, command, receipt); perr != nil {
			return a.failedAfterEffects(ctx, guard, command, fmt.Errorf("retired receipt not delivered: %w", perr))
		}
		// Final step of a VERIFIED retirement: release the port reservation.
		// The guard re-proves the consumed lease and the empty port itself.
		if rerr := guard.ReleaseVerified(ctx, ingressauthority.RetirementEvidenceV1{
			Port: evidence.Port, Owner: evidence.Owner,
			JournalRetired: evidence.Phase == ingressownership.PhaseRetired, RuleAbsent: evidence.RuleAbsent,
		}); rerr != nil {
			return "operational ingress retired; exact removal verified; reservation stays fenced: " + rerr.Error(), nil
		}
		return "operational ingress retired; exact removal verified; reservation released", nil
	}
}

// executorFailure maps an executor refusal. Known pre-effect refusals (the
// executor's own preflight) report BLOCKED; anything else is a failure after
// possible effects. Both durably record NEEDS_RECONCILIATION since the
// lifecycle was already APPLYING or leased.
func (a *Agent) executorFailure(ctx context.Context, guard *ingressauthority.PendingGuard, command ingressauthority.CommandV1, cause error) (string, error) {
	if errors.Is(cause, ingressinventory.ErrCoverageIncomplete) || errors.Is(cause, ingressinventory.ErrUnsupportedManagedNode) ||
		errors.Is(cause, ingressinventory.ErrPersistentServiceInventoryIncomplete) || errors.Is(cause, ingressinventory.ErrClaimConflict) ||
		errors.Is(cause, ingressownership.ErrExecutorTrustUnavailable) {
		rerr := a.recordReconciliation(ctx, guard, command)
		cctx, cancel := cleanupContext(ctx)
		defer cancel()
		return "operational ingress blocked", errors.Join(cause, rerr, a.postOperationalReceipt(cctx, command, blockedReceipt(command, cause)))
	}
	return a.failedAfterEffects(ctx, guard, command, cause)
}

func (a *Agent) failedAfterEffects(ctx context.Context, guard *ingressauthority.PendingGuard, command ingressauthority.CommandV1, cause error) (string, error) {
	rerr := a.recordReconciliation(ctx, guard, command)
	cctx, cancel := cleanupContext(ctx)
	defer cancel()
	receipt := ingressauthority.ReceiptV1{
		SchemaVersion: 1, JobID: command.JobID, Action: command.Action, Binding: command.Binding, Phase: "failed",
		FailureCause: "operational ingress outcome uncertain: " + cause.Error(), RollbackStatus: "reconciliation_required",
	}
	return "operational ingress needs reconciliation", errors.Join(cause, rerr, a.postOperationalReceipt(cctx, command, receipt))
}

func (a *Agent) recordReconciliation(ctx context.Context, guard *ingressauthority.PendingGuard, command ingressauthority.CommandV1) error {
	cctx, cancel := cleanupContext(ctx)
	defer cancel()
	return guard.RecordReconciliationNeeded(cctx, command)
}

// cleanupContext survives cancellation of the job context so an interrupt
// still durably records uncertainty and reports it.
func cleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), ingressCleanupTimeout)
}

func blockedReceipt(command ingressauthority.CommandV1, cause error) ingressauthority.ReceiptV1 {
	return ingressauthority.ReceiptV1{
		SchemaVersion: 1, JobID: command.JobID, Action: command.Action, Binding: command.Binding,
		Phase: "blocked", FailureCause: "operational ingress refused before host effects: " + cause.Error(), RollbackStatus: "not_attempted",
	}
}

func (a *Agent) postOperationalReceipt(ctx context.Context, command ingressauthority.CommandV1, receipt ingressauthority.ReceiptV1) error {
	if receipt.JobID != command.JobID || receipt.Binding != command.Binding {
		return ingressauthority.ErrBinding
	}
	if ctx.Err() != nil {
		var cancel context.CancelFunc
		ctx, cancel = cleanupContext(ctx)
		defer cancel()
	}
	return a.postIngressReceipt(ctx, receipt)
}
