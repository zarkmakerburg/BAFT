package bcc

import (
	"fmt"
	"regexp"
)

var ingressDigestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var ingressResourceIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

// PublicIngressExpectedProof is retained by BCC independently of an agent
// observation. It must come from a durable, authenticated operation cursor.
type PublicIngressExpectedProof struct {
	Generation     uint64
	ManifestDigest string
	ListenerID     string
	FirewallID     string
}

func (p PublicIngressExpectedProof) valid() bool {
	return p.Generation != 0 && ingressDigestPattern.MatchString(p.ManifestDigest) && ingressResourceIDPattern.MatchString(p.ListenerID) && ingressResourceIDPattern.MatchString(p.FirewallID) && p.ListenerID != p.FirewallID
}

// PublicIngressObserved is agent-reported evidence, not BCC's desired state.
type PublicIngressObserved struct {
	Spec          PublicIngressSpec
	Proof         PublicIngressExpectedProof
	ListenerReady bool
	FirewallReady bool
	OwnerVerified bool
}

// PublicIngressReconcileDecision is deliberately pure and never performs a
// host mutation. Only a signed agent action can apply a change.
type PublicIngressReconcileDecision string

const (
	PublicIngressNoop       PublicIngressReconcileDecision = "NOOP"
	PublicIngressNeedsApply PublicIngressReconcileDecision = "NEEDS_APPLY"
	PublicIngressBlocked    PublicIngressReconcileDecision = "BLOCKED"
)

// DecidePublicIngressReconcile fails closed when the agent reports an
// unowned listener or an inconsistent mapping on the requested port.
func DecidePublicIngressReconcile(desired PublicIngressSpec, expected PublicIngressExpectedProof, observed *PublicIngressObserved) (PublicIngressReconcileDecision, error) {
	canonicalDesired, err := PlanPublicIngress(desired.IRNode, desired.TunnelID, desired.PublicPort, desired.LocalTarget)
	if err != nil {
		return PublicIngressBlocked, err
	}
	if !expected.valid() {
		return PublicIngressBlocked, fmt.Errorf("missing or invalid durable ingress proof")
	}
	if observed == nil {
		return PublicIngressNeedsApply, nil
	}
	canonicalObserved, err := PlanPublicIngress(observed.Spec.IRNode, observed.Spec.TunnelID, observed.Spec.PublicPort, observed.Spec.LocalTarget)
	if err != nil {
		return PublicIngressBlocked, fmt.Errorf("invalid ingress observation: %w", err)
	}
	if canonicalObserved.IRNode != canonicalDesired.IRNode || canonicalObserved.PublicPort != canonicalDesired.PublicPort {
		return PublicIngressBlocked, fmt.Errorf("ingress observation does not match requested node and port")
	}
	if !observed.OwnerVerified {
		return PublicIngressBlocked, fmt.Errorf("ingress ownership cannot be verified")
	}
	if observed.Proof != expected {
		return PublicIngressBlocked, fmt.Errorf("ingress generation, digest or resource identity mismatch")
	}
	if canonicalObserved != canonicalDesired {
		return PublicIngressBlocked, fmt.Errorf("ingress ownership or local target differs from desired state")
	}
	if observed.ListenerReady && observed.FirewallReady {
		return PublicIngressNoop, nil
	}
	// A partial deployment needs explicit recovery before any new apply.
	// A blind retry could duplicate an existing listener or firewall rule.
	if observed.ListenerReady || observed.FirewallReady {
		return PublicIngressBlocked, fmt.Errorf("partial ingress resources require explicit recovery")
	}
	return PublicIngressNeedsApply, nil
}
