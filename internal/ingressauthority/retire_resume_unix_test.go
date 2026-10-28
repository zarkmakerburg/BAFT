//go:build unix

package ingressauthority

import "testing"

// Per-action command identity is immutable except for resuming a SPENT Retire
// (lease consumed, operation still NEEDS_RECONCILIATION) with a NEW job.
func TestMayResumeRetireOnlyForSpentRetireInNeedsReconciliation(t *testing.T) {
	b := bindingFixture(t)
	retire := func(id string) CommandV1 { return CommandV1{JobID: id, Action: commandIntentRetire, Binding: b} }
	apply := func(id string) CommandV1 { return CommandV1{JobID: id, Action: commandIntentApply, Binding: b} }
	spent := lifecycleEntry{Binding: b, State: StateNeedsReconciliation, Leased: map[string]string{commandIntentRetire: "r1"}}
	if !mayResumeRetire(spent, retire("r1"), retire("r2")) {
		t.Fatal("a new Retire must resume a spent Retire in NEEDS_RECONCILIATION")
	}
	other := b
	other.Port++
	moved := retire("r2")
	moved.Binding = other
	for name, c := range map[string]struct {
		e        lifecycleEntry
		old, new CommandV1
	}{
		"same job":        {spent, retire("r1"), retire("r1")},
		"empty job":       {spent, retire("r1"), retire("")},
		"apply replaced":  {lifecycleEntry{State: StateNeedsReconciliation, Leased: map[string]string{commandIntentApply: "a1"}}, apply("a1"), apply("a2")},
		"retire by apply": {spent, retire("r1"), apply("r2")},
		"unspent retire":  {lifecycleEntry{State: StateNeedsReconciliation}, retire("r1"), retire("r2")},
		"other job spent": {lifecycleEntry{State: StateNeedsReconciliation, Leased: map[string]string{commandIntentRetire: "x"}}, retire("r1"), retire("r2")},
		"clean applying":  {lifecycleEntry{State: StateApplying, Leased: map[string]string{commandIntentRetire: "r1"}}, retire("r1"), retire("r2")},
		"binding drift":   {spent, retire("r1"), moved},
	} {
		if mayResumeRetire(c.e, c.old, c.new) {
			t.Fatalf("%s: intent replacement allowed", name)
		}
	}
}
