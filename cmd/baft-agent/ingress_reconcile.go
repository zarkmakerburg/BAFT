package main

import (
	"context"
	"fmt"

	"github.com/zarkmakerburg/baft/internal/agent"
)

// parseReconcileMode validates --ingress-reconcile.
func parseReconcileMode(s string) (agent.RestartReconcileMode, error) {
	switch m := agent.RestartReconcileMode(s); m {
	case agent.RestartReconcileAuto, agent.RestartReconcileReport, agent.RestartReconcileOff:
		return m, nil
	}
	return "", fmt.Errorf("--ingress-reconcile must be auto, report or off, not %q", s)
}

// ingressRestarter is the Agent method the startup step needs.
type ingressRestarter interface {
	ReconcileIngressAfterRestart(context.Context, agent.RestartReconcileMode) ([]agent.IngressRestartResult, error)
}

// reconcileIngressAtStartup runs the post-restart reconciliation once, logs
// the exact evidence for every fenced reservation and returns true when at
// least one operation stays fenced (needs an operator). It is called after
// the operational executor was provisioned and before the poll loop, so no
// signed job can run while an interrupted one is unreconciled.
func reconcileIngressAtStartup(ctx context.Context, a ingressRestarter, mode agent.RestartReconcileMode, logf func(string, ...any)) bool {
	if mode == agent.RestartReconcileOff {
		return false
	}
	results, err := a.ReconcileIngressAfterRestart(ctx, mode)
	fenced := false
	for _, r := range results {
		fenced = fenced || r.Outcome == "fenced" || r.Outcome == ""
		logf("baft-agent: ingress restart reconciliation port=%d job=%s direction=%s outcome=%s receipt=%q: %s",
			r.Port, r.JobID, r.Direction, r.Outcome, r.Receipt, r.Reason)
		for _, s := range r.Report.Sockets {
			logf("baft-agent:   socket %s state=%s inode=%d uid=%d class=%s", s.Family, s.State, s.Inode, s.Uid, s.Class)
		}
	}
	if err != nil {
		fenced = true
		logf("baft-agent: ingress restart reconciliation left operations fenced: %v", err)
	}
	if len(results) == 0 {
		logf("baft-agent: ingress restart reconciliation: nothing fenced")
	}
	return fenced
}
