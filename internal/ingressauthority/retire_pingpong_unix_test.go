//go:build unix

package ingressauthority

import (
	"testing"
	"time"
)

// SECREVIEW: two validly signed, unexpired Retires must not be able to
// replace each other forever (r1 -> r2 -> r1 ...), re-running a spent job.
func TestSpentRetireCannotBeRevivedByPingPong(t *testing.T) {
	b := bindingFixture(t)
	now := time.Now().UTC()
	cmd := func(id, action string) CommandV1 {
		return CommandV1{IngressProtocol: ProtocolV1, JobID: id, Action: action,
			IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Binding: b}
	}
	entry := lifecycleEntry{Binding: b, State: StateNeedsReconciliation, Policy: &InstalledPolicyIdentityV1{},
		Commands: map[string]CommandV1{commandIntentApply: cmd("a1", commandIntentApply), commandIntentRetire: cmd("r1", commandIntentRetire)},
		Leased:   map[string]string{commandIntentRetire: "r1"}}
	entries := map[string]lifecycleEntry{reservationKey(b): entry}
	e2, same, err := planCommandRecord(entries, entry, cmd("r2", commandIntentRetire), now)
	if err != nil || same {
		t.Fatalf("legitimate resume refused: %v", err)
	}
	e2.Leased = map[string]string{commandIntentRetire: "r2"} // r2 minted and consumed
	entries[reservationKey(b)] = e2
	if _, _, err := planCommandRecord(entries, e2, cmd("r1", commandIntentRetire), now); err == nil {
		t.Fatal("spent Retire r1 revived by replaying it over r2 (ping-pong)")
	}
	if _, _, err := planCommandRecord(entries, e2, cmd("r2", commandIntentRetire), now); err != nil {
		t.Fatalf("exact retry of current intent must stay idempotent: %v", err)
	}
}
