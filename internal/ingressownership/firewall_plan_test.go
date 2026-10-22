package ingressownership

import (
	"errors"
	"testing"
)

func TestIngressRulePlanExactOwnerAndLocation(t *testing.T) {
	owner := proof("plan", 2)
	plan, err := PlanIngressRule(443, owner)
	if err != nil {
		t.Fatal(err)
	}
	repeat, err := PlanIngressRule(443, owner)
	if err != nil || repeat != plan {
		t.Fatalf("nondeterministic rule plan: %v", err)
	}
	good := FirewallObservation{Port: 443, Tag: plan.Tag, Handle: 19, Family: "inet", Table: "baft", Chain: "ingress"}
	if err := VerifyRuleObservation(plan, good); err != nil {
		t.Fatal(err)
	}
	cases := []FirewallObservation{{Port: 22, Tag: plan.Tag, Handle: 19, Family: "inet", Table: "baft", Chain: "ingress"}, {Port: 443, Tag: "foreign", Handle: 19, Family: "inet", Table: "baft", Chain: "ingress"}, {Port: 443, Tag: plan.Tag, Handle: 0, Family: "inet", Table: "baft", Chain: "ingress"}, {Port: 443, Tag: plan.Tag, Handle: 19, Family: "inet", Table: "foreign", Chain: "ingress"}}
	for _, c := range cases {
		if err := VerifyRuleObservation(plan, c); !errors.Is(err, ErrFirewallUnverified) {
			t.Fatalf("unverified observation accepted: %+v (%v)", c, err)
		}
	}
	if _, err := PlanIngressRule(22, owner); !errors.Is(err, ErrInvalid) {
		t.Fatalf("SSH port admitted: %v", err)
	}
	other := proof("plan", 3)
	otherPlan, err := PlanIngressRule(443, other)
	if err != nil {
		t.Fatal(err)
	}
	if otherPlan.Tag == plan.Tag {
		t.Fatal("generation not bound to rule tag")
	}
}
