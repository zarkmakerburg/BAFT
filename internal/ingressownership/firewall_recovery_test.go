package ingressownership

import (
	"errors"
	"testing"
)

func TestLABFirewallRemovalRejectsHandleSwapAndForeignOwner(t *testing.T) {
	plan, err := PlanIngressRule(443, proof("recover", 9))
	if err != nil {
		t.Fatal(err)
	}
	before := FirewallObservation{Port: 443, Tag: plan.Tag, Handle: 11, Family: "inet", Table: "baft", Chain: "ingress"}
	decision, err := InspectFirewallRecovery(plan, []FirewallObservation{before})
	if err != nil || !decision.Verified || decision.Owned != before {
		t.Fatalf("unexpected recovery decision: %+v %v", decision, err)
	}
	if err := VerifyFirewallRemovalFence(plan, before, []FirewallObservation{before}); err != nil {
		t.Fatal(err)
	}
	swapped := before
	swapped.Handle = 12
	foreign := before
	foreign.Tag = "foreign"
	for _, observations := range [][]FirewallObservation{{swapped}, {foreign}, {before, swapped}, nil} {
		if err := VerifyFirewallRemovalFence(plan, before, observations); !errors.Is(err, ErrFirewallUnverified) {
			t.Fatalf("unsafe removal allowed: %+v %v", observations, err)
		}
	}
	if _, err := InspectFirewallRecovery(plan, nil); !errors.Is(err, ErrFirewallUnverified) {
		t.Fatalf("missing firewall adopted: %v", err)
	}
}
