package agent

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressownership"
)

// RestartReconcileMode selects how the startup step runs.
type RestartReconcileMode string

const (
	// RestartReconcileAuto evaluates every fenced operation and finishes the
	// ones whose kernel/firewall/authority evidence is complete.
	RestartReconcileAuto RestartReconcileMode = "auto"
	// RestartReconcileReport evaluates the same evidence and writes nothing.
	RestartReconcileReport RestartReconcileMode = "report"
	// RestartReconcileOff does nothing (operations stay fenced).
	RestartReconcileOff RestartReconcileMode = "off"
)

// ingressRestartSource is implemented by *ingressownership.OperationalExecutor.
type ingressRestartSource interface {
	RestartReconciler() *ingressownership.RestartReconciler
}

// IngressRestartResult is the outcome for one fenced operation.
type IngressRestartResult struct {
	Port      uint16
	JobID     string
	Direction ingressauthority.RestartDirection
	Outcome   ingressownership.RestartOutcome
	Reason    string
	Report    ingressownership.RestartReport
	// Receipt is "posted", "not-posted: <why>" or "" (none attempted).
	Receipt string
}

// ReconcileIngressAfterRestart runs once at Agent startup, before the poll
// loop. For every durable reservation that survived the restart it takes the
// authority lock, derives the remove-only restart authority from the
// lifecycle (a CONSUMED signed lease is required; no new signed job, no new
// lease), and asks the reconciler to finish the interrupted effect ONLY when
// the kernel socket table, the pinned nft chain and the journal all agree
// (see ingressownership/restart_reconcile.go). Anything else is reported and
// stays fenced exactly as before.
//
// It never creates a listener or a rule and never mints a lease. A completed
// reconciliation can release its reservation only after verified retirement,
// absent-rule readback and a fresh, complete socket inventory. BCC is told through
// the existing authenticated receipt path with one of the already-defined
// shapes ("retired" for a completed Retire, "failed/reconciliation_required"
// for a rolled-back Apply). That is best effort: BCC refuses receipts outside
// the signed command's lifetime or that conflict with a stored one, and a
// failed delivery never changes the local, already-durable outcome.
func (a *Agent) ReconcileIngressAfterRestart(ctx context.Context, mode RestartReconcileMode) ([]IngressRestartResult, error) {
	if a == nil || ctx == nil {
		return nil, ingressauthority.ErrUnverified
	}
	op := a.cfg.IngressOperational
	if op == nil || op.Store == nil || op.Executor == nil || mode == RestartReconcileOff || mode == "" {
		return nil, nil
	}
	src, ok := op.Executor.(ingressRestartSource)
	if !ok {
		return nil, nil
	}
	rec := src.RestartReconciler()
	if rec == nil {
		return nil, nil
	}
	rec.DryRun = mode == RestartReconcileReport
	leftovers, err := op.Store.InspectFencedLeftovers()
	if err != nil {
		return nil, fmt.Errorf("reservation lifecycle scan: %w", err)
	}
	sort.Slice(leftovers, func(i, j int) bool { return leftovers[i].Binding.Port < leftovers[j].Binding.Port })
	var results []IngressRestartResult
	var errs []error
	for _, l := range leftovers {
		if l.Binding.NodeID != a.cfg.NodeID {
			results = append(results, IngressRestartResult{Port: l.Binding.Port, Outcome: ingressownership.OutcomeFenced, Reason: "reservation belongs to another node"})
			continue
		}
		res := a.reconcileOne(ctx, op.Store, rec, l.Binding, mode)
		if res.Outcome == ingressownership.OutcomeFenced {
			errs = append(errs, fmt.Errorf("port %d: %s", res.Port, res.Reason))
		}
		results = append(results, res)
	}
	return results, errors.Join(errs...)
}

func (a *Agent) reconcileOne(ctx context.Context, store *ingressauthority.ReservationStore, rec *ingressownership.RestartReconciler, b ingressauthority.BindingV1, mode RestartReconcileMode) IngressRestartResult {
	res := IngressRestartResult{Port: b.Port, Outcome: ingressownership.OutcomeFenced}
	guard, err := store.HoldPending(b)
	if err != nil {
		res.Reason = "authority lock unavailable (another agent may be running): " + err.Error()
		return res
	}
	defer guard.Close()
	auth, err := guard.RestartAuthority(ctx)
	if err != nil {
		res.Reason = "no restart authority (no consumed signed lease on record, or policy/claim drift): " + err.Error()
		return res
	}
	res.JobID, res.Direction = auth.Command().JobID, auth.Direction()
	report, rerr := rec.Reconcile(ctx, auth)
	res.Report, res.Outcome, res.Reason = report, report.Outcome, report.Reason
	if rerr != nil {
		res.Outcome = ingressownership.OutcomeFenced
		if res.Reason == "" {
			res.Reason = rerr.Error()
		}
		return res
	}
	if mode == RestartReconcileReport {
		return res
	}
	switch report.Outcome {
	case ingressownership.OutcomeRolledBack, ingressownership.OutcomeRetired, ingressownership.OutcomeAlreadyRetired, ingressownership.OutcomeNoEffects:
	default:
		return res
	}
	// The host is reconciled. Record that in the lifecycle (state only).
	if err := guard.RecordRestartReconciled(ctx, auth); err != nil {
		res.Reason += "; lifecycle state not recorded: " + err.Error()
		return res
	}
	res.Receipt = a.postRestartReceipt(ctx, auth, report)
	// A completion that satisfies the same evidence as a verified Retire
	// (journal retired, rule read back absent, port empty) releases the port.
	if report.Outcome != ingressownership.OutcomeNoEffects {
		ev := ingressauthority.RetirementEvidenceV1{Port: b.Port, Owner: b.Proof,
			JournalRetired: report.JournalPhase == ingressownership.PhaseRetired, RuleAbsent: true}
		if err := guard.ReleaseVerified(ctx, ev); err != nil {
			res.Reason += "; reservation stays fenced: " + err.Error()
		} else {
			res.Reason += "; reservation released"
		}
	}
	return res
}

// postRestartReceipt fits the outcome into an existing receipt shape or says
// that BCC stays unaware.
func (a *Agent) postRestartReceipt(ctx context.Context, auth *ingressauthority.RestartAuthority, report ingressownership.RestartReport) string {
	cmd := auth.Command()
	var receipt ingressauthority.ReceiptV1
	switch {
	case auth.Direction() == ingressauthority.RestartRetire &&
		(report.Outcome == ingressownership.OutcomeRetired || report.Outcome == ingressownership.OutcomeAlreadyRetired):
		receipt = ingressauthority.ReceiptV1{SchemaVersion: 1, JobID: cmd.JobID, Action: cmd.Action, Binding: cmd.Binding, Phase: "retired"}
	case auth.Direction() == ingressauthority.RestartApply && report.Outcome == ingressownership.OutcomeRolledBack:
		receipt = ingressauthority.ReceiptV1{SchemaVersion: 1, JobID: cmd.JobID, Action: cmd.Action, Binding: cmd.Binding, Phase: "failed",
			FailureCause: "operational ingress interrupted by an agent restart; the owned rule was removed after kernel evidence (reservation stays fenced)", RollbackStatus: "reconciliation_required"}
	default:
		return "not-posted: no existing receipt shape fits outcome " + string(report.Outcome)
	}
	if err := a.postOperationalReceipt(ctx, cmd, receipt); err != nil {
		return "not-posted: " + err.Error()
	}
	return "posted"
}
