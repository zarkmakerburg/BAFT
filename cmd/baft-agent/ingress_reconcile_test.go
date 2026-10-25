package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/zarkmakerburg/baft/internal/agent"
	"github.com/zarkmakerburg/baft/internal/ingressownership"
)

type fakeRestarter struct {
	calls   []agent.RestartReconcileMode
	results []agent.IngressRestartResult
	err     error
}

func (f *fakeRestarter) ReconcileIngressAfterRestart(_ context.Context, m agent.RestartReconcileMode) ([]agent.IngressRestartResult, error) {
	f.calls = append(f.calls, m)
	return f.results, f.err
}

func TestParseReconcileModeAcceptsOnlyTheThreeModes(t *testing.T) {
	for _, ok := range []string{"auto", "report", "off"} {
		if _, err := parseReconcileMode(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "AUTO", "force", "yes", "auto "} {
		if _, err := parseReconcileMode(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	var stderr bytes.Buffer
	if got := run(context.Background(), []string{"--bcc-url", "https://x", "--node-id", "n1", "--ingress-reconcile", "force"}, &stderr); got != 2 {
		t.Fatalf("invalid mode exit %d: %s", got, stderr.String())
	}
	stderr.Reset()
	if got := run(context.Background(), []string{"--bcc-url", "https://x", "--node-id", "n1", "--reconcile-ingress", "--ingress-reconcile", "off"}, &stderr); got != 2 {
		t.Fatalf("contradictory flags exit %d: %s", got, stderr.String())
	}
}

func TestStartupReconcileFencedOnlyWhenSomethingStaysFenced(t *testing.T) {
	var logs []string
	logf := func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) }
	off := &fakeRestarter{}
	if reconcileIngressAtStartup(context.Background(), off, agent.RestartReconcileOff, logf) || len(off.calls) != 0 {
		t.Fatal("mode off must not even look")
	}
	done := &fakeRestarter{results: []agent.IngressRestartResult{{Port: 2053, JobID: "j1", Direction: "retire", Outcome: ingressownership.OutcomeRetired, Reason: "ok", Receipt: "posted"}}}
	if reconcileIngressAtStartup(context.Background(), done, agent.RestartReconcileAuto, logf) {
		t.Fatal("a reconciled operation was reported as fenced")
	}
	held := &fakeRestarter{results: []agent.IngressRestartResult{{Port: 2053, Outcome: ingressownership.OutcomeFenced, Reason: "foreign listener"}}, err: errors.New("port 2053: foreign listener")}
	if !reconcileIngressAtStartup(context.Background(), held, agent.RestartReconcileReport, logf) {
		t.Fatal("a fenced operation was reported as clean")
	}
	if len(held.calls) != 1 || held.calls[0] != agent.RestartReconcileReport {
		t.Fatalf("mode not passed through: %v", held.calls)
	}
	if !strings.Contains(strings.Join(logs, "\n"), "foreign listener") {
		t.Fatalf("evidence not logged: %v", logs)
	}
	boom := &fakeRestarter{err: errors.New("scan failed")}
	if !reconcileIngressAtStartup(context.Background(), boom, agent.RestartReconcileAuto, logf) {
		t.Fatal("a failed scan must read as fenced")
	}
}
