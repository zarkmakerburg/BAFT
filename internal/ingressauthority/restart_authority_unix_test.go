//go:build unix

package ingressauthority

import (
	"testing"
	"time"
)

func restartEntry(t *testing.T, leased map[string]string, retireID string, superseded ...string) lifecycleEntry {
	t.Helper()
	b := bindingFixture(t)
	now := time.Now().UTC()
	cmd := func(id, action string) CommandV1 {
		return CommandV1{IngressProtocol: ProtocolV1, JobID: id, Action: action,
			IssuedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Minute), Binding: b} // long expired: irrelevant to remove-only recovery
	}
	e := lifecycleEntry{Binding: b, State: StateApplying, Policy: &InstalledPolicyIdentityV1{},
		Commands:   map[string]CommandV1{commandIntentApply: cmd("a1", commandIntentApply)},
		Leased:     leased,
		Superseded: superseded}
	if retireID != "" {
		e.Commands[commandIntentRetire] = cmd(retireID, commandIntentRetire)
	}
	return e
}

// Only a CONSUMED lease justifies post-restart cleanup. A merely recorded
// (admitted but never leased) intent never does, however fresh or stale.
func TestRestartDirectionRequiresAConsumedLease(t *testing.T) {
	if _, _, err := restartDirection(restartEntry(t, nil, "")); err == nil {
		t.Fatal("an unleased Apply intent authorized restart cleanup")
	}
	if _, _, err := restartDirection(restartEntry(t, nil, "r1")); err == nil {
		t.Fatal("an unleased Retire intent authorized restart cleanup")
	}
	dir, cmd, err := restartDirection(restartEntry(t, map[string]string{commandIntentApply: "a1"}, ""))
	if err != nil || dir != RestartApply || cmd.JobID != "a1" {
		t.Fatalf("apply: %v %v %+v", dir, err, cmd)
	}
	dir, cmd, err = restartDirection(restartEntry(t, map[string]string{commandIntentApply: "a1", commandIntentRetire: "r1"}, "r1"))
	if err != nil || dir != RestartRetire || cmd.JobID != "r1" {
		t.Fatalf("retire wins over apply: %v %v %+v", dir, err, cmd)
	}
}

// A lease that does not match the stored intent (stale, swapped or forged
// lifecycle data) and any spent (superseded) Retire never authorize anything.
func TestRestartDirectionDeniesMismatchedAndSupersededJobs(t *testing.T) {
	if _, _, err := restartDirection(restartEntry(t, map[string]string{commandIntentRetire: "r2"}, "r1")); err == nil {
		t.Fatal("lease for a different job than the stored Retire intent authorized cleanup")
	}
	if _, _, err := restartDirection(restartEntry(t, map[string]string{commandIntentRetire: "ghost"}, "")); err == nil {
		t.Fatal("lease without any stored intent authorized cleanup")
	}
	if _, _, err := restartDirection(restartEntry(t, map[string]string{commandIntentRetire: "r1"}, "r1", "r1")); err == nil {
		t.Fatal("a superseded (spent) Retire authorized cleanup")
	}
	e := restartEntry(t, map[string]string{commandIntentApply: "a1"}, "")
	other := e.Binding
	other.Port++
	c := e.Commands[commandIntentApply]
	c.Binding = other
	e.Commands[commandIntentApply] = c
	if _, _, err := restartDirection(e); err == nil {
		t.Fatal("intent bound to another resource authorized cleanup")
	}
}

// Regression for the Retire ping-pong class (v0.4.0 security review #1):
// deriving a restart authority must not make a spent job recordable again.
// r1 was leased then superseded by r2; r2 was leased. After a restart the
// authority is derived from r2 only, r1 stays spent, and a replay of r1 over
// r2 is still refused by the admission plan.
func TestRestartAuthorityDoesNotReviveSupersededRetire(t *testing.T) {
	b := bindingFixture(t)
	now := time.Now().UTC()
	cmd := func(id, action string) CommandV1 {
		return CommandV1{IngressProtocol: ProtocolV1, JobID: id, Action: action,
			IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Binding: b}
	}
	e := lifecycleEntry{Binding: b, State: StateNeedsReconciliation, Policy: &InstalledPolicyIdentityV1{},
		Commands:   map[string]CommandV1{commandIntentApply: cmd("a1", commandIntentApply), commandIntentRetire: cmd("r2", commandIntentRetire)},
		Leased:     map[string]string{commandIntentApply: "a1", commandIntentRetire: "r2"},
		Superseded: []string{"r1"}}
	dir, got, err := restartDirection(e)
	if err != nil || dir != RestartRetire || got.JobID != "r2" {
		t.Fatalf("authority: %v %v %+v", dir, err, got)
	}
	entries := map[string]lifecycleEntry{reservationKey(b): e}
	if _, _, err := planCommandRecord(entries, e, cmd("r1", commandIntentRetire), now); err == nil {
		t.Fatal("spent Retire r1 became recordable again")
	}
	// The spent r2 itself cannot be replaced by a lookalike either: the
	// restart path never writes Leased/Commands/Superseded, only State.
	if len(e.Superseded) != 1 || e.Leased[commandIntentRetire] != "r2" {
		t.Fatal("restartDirection mutated the lifecycle entry")
	}
}
