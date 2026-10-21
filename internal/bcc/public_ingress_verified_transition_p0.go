package bcc

import "fmt"

// TransitionVerifiedPublicIngressApplied combines the independent reconcile
// proof check with the lifecycle transition. It is a pure gate: the caller
// must obtain an authenticated fresh agent observation and a durable expected
// proof. No listener or firewall is created here.
func TransitionVerifiedPublicIngressApplied(current PublicIngressLifecycle, expected PublicIngressExpectedProof, observed *PublicIngressObserved) (PublicIngressLifecycle, error) {
	decision, err := DecidePublicIngressReconcile(current.Spec, expected, observed)
	if err != nil {
		return current, err
	}
	if decision != PublicIngressNoop {
		return current, fmt.Errorf("public ingress cannot be marked applied without verified listener and firewall: %s", decision)
	}
	return TransitionPublicIngress(current, current.Spec, PublicIngressApplied)
}
