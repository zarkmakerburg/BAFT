package ingressownership

// FirewallRecoveryDecision is a read-only reconciliation result for LAB.
// No decision grants permission to remove a kernel rule without rechecking
// its exact handle and owner immediately before the mutation.
type FirewallRecoveryDecision struct {
	Owned    FirewallObservation
	Verified bool
}

// InspectFirewallRecovery never adopts an active rule on a missing or
// ambiguous observation. The caller must separately verify the live socket.
func InspectFirewallRecovery(plan RulePlan, observations []FirewallObservation) (FirewallRecoveryDecision, error) {
	observed, err := ReconcileRuleObservation(plan, observations)
	if err != nil {
		return FirewallRecoveryDecision{}, err
	}
	return FirewallRecoveryDecision{Owned: observed, Verified: true}, nil
}

// VerifyFirewallRemovalFence requires an unchanged kernel handle and exact
// identity across the preflight and final observation. This is an inert
// policy gate, not a production nftables adapter.
func VerifyFirewallRemovalFence(plan RulePlan, before FirewallObservation, latest []FirewallObservation) error {
	if err := VerifyRuleObservation(plan, before); err != nil {
		return err
	}
	current, err := ReconcileRuleObservation(plan, latest)
	if err != nil {
		return err
	}
	if current != before {
		return ErrFirewallUnverified
	}
	return nil
}
