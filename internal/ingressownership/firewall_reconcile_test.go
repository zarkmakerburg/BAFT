package ingressownership

import (
	"errors"
	"testing"
)

func TestReconcileRuleObservationFailsClosed(t *testing.T) {
	plan, err := PlanIngressRule(443, proof("reconcile", 7))
	if err != nil {
		t.Fatal(err)
	}
	owned := FirewallObservation{Port: 443, Tag: plan.Tag, Handle: 17, Family: "inet", Table: "baft", Chain: "ingress"}
	unrelated := FirewallObservation{Port: 8443, Tag: "other", Handle: 18, Family: "inet", Table: "baft", Chain: "ingress"}
	got, err := ReconcileRuleObservation(plan, []FirewallObservation{unrelated, owned})
	if err != nil || got != owned {
		t.Fatalf("valid rule rejected: %v", err)
	}
	bad := [][]FirewallObservation{nil, {unrelated}, {owned, owned}, {{Port: 443, Tag: "foreign", Handle: 12, Family: "inet", Table: "baft", Chain: "ingress"}, owned}, {owned, {Port: 8443, Tag: plan.Tag, Handle: 19, Family: "inet", Table: "baft", Chain: "ingress"}}}
	for i, observations := range bad {
		if _, err := ReconcileRuleObservation(plan, observations); !errors.Is(err, ErrFirewallUnverified) {
			t.Fatalf("case %d accepted: %v", i, err)
		}
	}
}
