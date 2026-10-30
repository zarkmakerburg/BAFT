//go:build unix

package ingressownership

import (
	"context"
	"errors"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressinventory"
)

func TestRestartRefusesDeleteWhenAuthorityLostDuringFinalInventory(t *testing.T) {
	r := newRestartRig(t, ingressauthority.RestartRetire)
	handle := r.nft.addOwned(r.plan.Tag, int(r.plan.Port))
	r.journalAt(PhaseRetireIntent, handle)
	reads := 0
	r.rec.Collect = func(context.Context, string, uint16) (ingressinventory.CoverageEvidence, error) {
		reads++
		if reads == 2 {
			r.auth.lost = true
		}
		return r.cov, nil
	}
	report, err := r.run()
	if !errors.Is(err, ErrRestartFenced) || report.Outcome != OutcomeFenced {
		t.Fatalf("lost authority was not refused: report=%+v err=%v", report, err)
	}
	if reads != 2 {
		t.Fatalf("did not exercise the final inventory: reads=%d", reads)
	}
	if len(r.nft.commands) != 0 {
		t.Fatalf("firewall mutation after authority loss: %v", r.nft.commands)
	}
	if _, present := r.nft.rules[handle]; !present {
		t.Fatal("owned rule deleted after authority loss")
	}
	if phase := r.phase(); phase != PhaseRetireIntent {
		t.Fatalf("journal advanced after authority loss: %s", phase)
	}
}

func TestRestartRefusesDeleteWhenForeignListenerArrivesDuringAuthorityValidation(t *testing.T) {
	for _, family := range []string{"tcp4", "tcp6"} {
		t.Run(family, func(t *testing.T) {
			r := newRestartRig(t, ingressauthority.RestartRetire)
			handle := r.nft.addOwned(r.plan.Tag, int(r.plan.Port))
			r.journalAt(PhaseRetireIntent, handle)
			entered := false
			r.auth.beforeEffect = func() {
				entered = true
				if r.collect != 2 {
					t.Fatalf("effect boundary preceded final outer inventory: reads=%d", r.collect)
				}
				r.rows = []ingressinventory.ActiveTCPClaim{row(family, "0A", 994, 33, r.plan.Port)}
			}
			report, err := r.run()
			if !entered {
				t.Fatal("did not reach the authority-validation boundary")
			}
			if !errors.Is(err, ErrRestartFenced) || report.Outcome != OutcomeFenced {
				t.Fatalf("late foreign listener was not refused: report=%+v err=%v", report, err)
			}
			if len(r.nft.commands) != 0 {
				t.Fatalf("firewall mutation despite late foreign listener: %v", r.nft.commands)
			}
			if _, present := r.nft.rules[handle]; !present {
				t.Fatal("owned rule deleted after a foreign listener arrived")
			}
			if phase := r.phase(); phase != PhaseRetireIntent {
				t.Fatalf("journal advanced despite a foreign listener: %s", phase)
			}
		})
	}
}
