package bcc

// TransitionVerifiedPublicIngressRemoved checks exact resource ownership and
// durable generation/digest proof before changing an applied lifecycle state.
// This is a pure authorization gate, NOT evidence of host deletion; the caller
// must independently verify host removal before invoking it.
func TransitionVerifiedPublicIngressRemoved(current PublicIngressLifecycle, expected PublicIngressExpectedProof, observed PublicIngressRetireProof) (PublicIngressLifecycle, error) {
	if err := CanRetirePublicIngress(current.Spec, expected, observed); err != nil {
		return current, err
	}
	return TransitionPublicIngress(current, current.Spec, PublicIngressRemoved)
}
