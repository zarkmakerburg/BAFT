package bcc

import "testing"

func TestP0VerifiedIngressRemovedTransition(t *testing.T) {
	spec, err := PlanPublicIngress("arno", "tun-a", 54619, "127.0.0.1:1444")
	if err != nil {
		t.Fatal(err)
	}
	applied := PublicIngressLifecycle{Spec: spec, Status: PublicIngressApplied, Applied: true}
	ready := PublicIngressRetireProof{Spec: spec, Proof: testIngressProof, OwnerVerified: true}
	next, err := TransitionVerifiedPublicIngressRemoved(applied, testIngressProof, ready)
	if err != nil || next.Status != PublicIngressRemoved || next.Applied {
		t.Fatalf("valid retire denied: %+v %v", next, err)
	}
	for _, tc := range []struct {
		name     string
		proof    PublicIngressRetireProof
		expected PublicIngressExpectedProof
	}{
		{"owner unverified", PublicIngressRetireProof{Spec: spec, Proof: testIngressProof}, testIngressProof},
		{"missing durable expected proof", ready, PublicIngressExpectedProof{}},
		{"wrong tunnel", PublicIngressRetireProof{Spec: PublicIngressSpec{IRNode: spec.IRNode, TunnelID: "other", PublicPort: spec.PublicPort, LocalTarget: spec.LocalTarget}, Proof: testIngressProof, OwnerVerified: true}, testIngressProof},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, e := TransitionVerifiedPublicIngressRemoved(applied, tc.expected, tc.proof)
			if e == nil || got != applied {
				t.Fatalf("unsafe retire: %+v %v", got, e)
			}
		})
	}
	stale := ready
	stale.Proof.Generation++
	if got, e := TransitionVerifiedPublicIngressRemoved(applied, testIngressProof, stale); e == nil || got != applied {
		t.Fatalf("stale retire: %+v %v", got, e)
	}
	if got, e := TransitionVerifiedPublicIngressRemoved(next, testIngressProof, ready); e == nil || got != next {
		t.Fatalf("replayed retire: %+v %v", got, e)
	}
}
