package bcc

import "fmt"

// PublicIngressRetireProof identifies exactly the managed resource that may
// be removed. A real agent must independently verify the live generation and
// ownership before performing any host mutation.
type PublicIngressRetireProof struct {
	Spec          PublicIngressSpec
	Proof         PublicIngressExpectedProof
	OwnerVerified bool
}

// CanRetirePublicIngress is a pure safety gate, not a deletion operation.
// It refuses an unknown owner, zero generation or stale generation.
func CanRetirePublicIngress(desired PublicIngressSpec, expected PublicIngressExpectedProof, proof PublicIngressRetireProof) error {
	canonicalDesired, err := PlanPublicIngress(desired.IRNode, desired.TunnelID, desired.PublicPort, desired.LocalTarget)
	if err != nil {
		return err
	}
	canonicalProof, err := PlanPublicIngress(proof.Spec.IRNode, proof.Spec.TunnelID, proof.Spec.PublicPort, proof.Spec.LocalTarget)
	if err != nil {
		return fmt.Errorf("invalid public ingress retire proof: %w", err)
	}
	if !proof.OwnerVerified {
		return fmt.Errorf("public ingress retire requires verified ownership")
	}
	if !expected.valid() || proof.Proof != expected {
		return fmt.Errorf("public ingress retire generation, digest or resource identity mismatch")
	}
	if canonicalDesired != canonicalProof {
		return fmt.Errorf("public ingress retire resource mismatch")
	}
	return nil
}
