package bcc

import "fmt"

// PublicIngressRecoveryDecision describes the next safe manual recovery step.
// It is advisory only; a signed agent inspection and explicit owned-resource
// executor are required before mutating any listener or firewall rule.
type PublicIngressRecoveryDecision string

const (
	IngressRecoveryNoop    PublicIngressRecoveryDecision = "NOOP"
	IngressRecoveryInspect PublicIngressRecoveryDecision = "INSPECT"
	IngressRecoveryBlocked PublicIngressRecoveryDecision = "BLOCKED"
)

// PlanPublicIngressRecovery never authorizes a blind retry or teardown.
func PlanPublicIngressRecovery(desired PublicIngressSpec, expected PublicIngressExpectedProof, observed *PublicIngressObserved) (PublicIngressRecoveryDecision, error) {
	decision, err := DecidePublicIngressReconcile(desired, expected, observed)
	if err != nil {
		if observed != nil && observed.OwnerVerified && observed.Proof == expected && expected.valid() {
			canonical, validation := PlanPublicIngress(observed.Spec.IRNode, observed.Spec.TunnelID, observed.Spec.PublicPort, observed.Spec.LocalTarget)
			wanted, wantedErr := PlanPublicIngress(desired.IRNode, desired.TunnelID, desired.PublicPort, desired.LocalTarget)
			if validation == nil && wantedErr == nil && canonical == wanted && (observed.ListenerReady != observed.FirewallReady) {
				return IngressRecoveryInspect, nil
			}
		}
		return IngressRecoveryBlocked, err
	}
	if decision == PublicIngressNoop {
		return IngressRecoveryNoop, nil
	}
	return IngressRecoveryBlocked, fmt.Errorf("recovery requires confirmed partial owned resources, not an unobserved or empty state")
}
