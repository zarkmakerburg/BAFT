package ingressownership

import (
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// RulePlan is an immutable, inert LAB representation of a narrowly scoped
// ingress rule. It never executes commands or mutates the host firewall.
// A future Linux adapter must verify kernel rule handles and ownership before
// applying or removing any rule; this plan is NOT that adapter.
type RulePlan struct {
	Port  uint16
	Owner ingressmanifest.Proof
	Tag   string
}

// PlanIngressRule rejects SSH/22 and non-validated identities, and creates a
// stable owner-specific tag for exact-match reconciliation.
func PlanIngressRule(port uint16, owner ingressmanifest.Proof) (RulePlan, error) {
	if port == 0 || port == 22 || !valid(owner) {
		return RulePlan{}, ErrInvalid
	}
	return RulePlan{Port: port, Owner: owner, Tag: ingressauthority.FirewallOwnerTagV1(port, owner)}, nil
}

// FirewallObservation is an independently captured kernel observation. An
// unverified or ambiguous handle must never authorize a destructive action.
type FirewallObservation struct {
	Port   uint16
	Tag    string
	Handle uint64
	Family string
	Table  string
	Chain  string
}

// VerifyRuleObservation is a pure LAB policy gate. It deliberately rejects
// unknown rule locations and zero handles rather than guessing ownership.
func VerifyRuleObservation(plan RulePlan, observed FirewallObservation) error {
	expected, err := PlanIngressRule(plan.Port, plan.Owner)
	if err != nil || expected.Tag != plan.Tag {
		return ErrInvalid
	}
	if observed.Port != plan.Port || observed.Tag != plan.Tag || observed.Handle == 0 || observed.Family != "inet" || observed.Table != "baft" || observed.Chain != "ingress" {
		return ErrFirewallUnverified
	}
	return nil
}

// ReconcileRuleObservation accepts exactly one matching rule and rejects any
// competing claim on the same port or owner tag. This is LAB-only.
func ReconcileRuleObservation(plan RulePlan, observations []FirewallObservation) (FirewallObservation, error) {
	expected, err := PlanIngressRule(plan.Port, plan.Owner)
	if err != nil || expected.Tag != plan.Tag {
		return FirewallObservation{}, ErrInvalid
	}
	var match FirewallObservation
	count := 0
	for _, o := range observations {
		if o.Port != plan.Port && o.Tag != plan.Tag {
			continue
		}
		if VerifyRuleObservation(plan, o) != nil {
			return FirewallObservation{}, ErrFirewallUnverified
		}
		count++
		if count > 1 {
			return FirewallObservation{}, ErrFirewallUnverified
		}
		match = o
	}
	if count != 1 {
		return FirewallObservation{}, ErrFirewallUnverified
	}
	return match, nil
}
