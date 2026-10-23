package bcc

import "fmt"

// TransitionVerifiedPublicIngressRemoved checks exact resource ownership and
// durable generation/digest proof before changing an applied lifecycle state.
// This is a pure authorization gate, NOT a host deletion. Both absence flags
// must come from an authenticated fresh inspection after removal.
func TransitionVerifiedPublicIngressRemoved(current PublicIngressLifecycle, expected PublicIngressExpectedProof, observed PublicIngressRetireProof, listenerAbsent, firewallAbsent bool) (PublicIngressLifecycle, error) {
	if err := CanRetirePublicIngress(current.Spec, expected, observed); err != nil {
		return current, err
	}
	if !listenerAbsent || !firewallAbsent {
		return current, fmt.Errorf("public ingress cannot be marked removed without verified listener and firewall absence")
	}
	return TransitionPublicIngress(current, current.Spec, PublicIngressRemoved)
}
