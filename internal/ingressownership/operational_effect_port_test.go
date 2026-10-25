package ingressownership

import (
	"context"
	"errors"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressinventory"
)

// Ported from hq's retiredPortObservation matrix ("wrong-port" row) to this
// branch's per-effect fence: a fresh inventory taken "at the effect" must be
// about the numeric port being mutated. Evidence for another port (a
// collector bug or a stale/aliased observation) proves nothing about this one
// and must deny the nft effect before it runs.
func TestFencedNFTRemoveRefusesEvidenceForAnotherPort(t *testing.T) {
	f := newPartialRetireFixture(t)
	b := f.cmd.Binding
	other := f.evidence()
	other.Port++
	fence := f.fence(func(context.Context, string, uint16) (ingressinventory.CoverageEvidence, error) { return other, nil })
	before := len(f.fake.commands)
	err := f.tx.retireLocked(context.Background(), b.Spec, b.Proof, fence)
	if err == nil || !errors.Is(err, ingressinventory.ErrCoverageIncomplete) {
		t.Fatalf("remove accepted evidence about another port: %v", err)
	}
	if len(f.fake.commands) != before || f.fake.ruleHandle == 0 {
		t.Fatalf("nft effect ran on evidence for another port: commands=%v rule=%d", f.fake.commands, f.fake.ruleHandle)
	}
	cur, _, _ := f.tx.Journal.Current(b.Proof)
	if cur.Phase != PhaseRetireIntent {
		t.Fatalf("refusal lost uncertainty: %+v", cur)
	}
}
