package bcc

import "testing"

func TestP0VerifiedIngressAppliedTransition(t *testing.T) {
	spec, err := PlanPublicIngress("arno", "tun-a", 54619, "127.0.0.1:1444")
	if err != nil {
		t.Fatal(err)
	}
	pending := PublicIngressLifecycle{Spec: spec, Status: PublicIngressPending}
	ready := &PublicIngressObserved{Spec: spec, Proof: testIngressProof, OwnerVerified: true, ListenerReady: true, FirewallReady: true}
	got, err := TransitionVerifiedPublicIngressApplied(pending, testIngressProof, ready)
	if err != nil || !got.Applied || got.Status != PublicIngressApplied {
		t.Fatalf("verified transition: %+v %v", got, err)
	}
	variants := []struct {
		name     string
		observed *PublicIngressObserved
		expected PublicIngressExpectedProof
	}{
		{"missing observation", nil, testIngressProof},
		{"missing proof", ready, PublicIngressExpectedProof{}},
		{"unverified owner", &PublicIngressObserved{Spec: spec, Proof: testIngressProof, ListenerReady: true, FirewallReady: true}, testIngressProof},
		{"missing listener", &PublicIngressObserved{Spec: spec, Proof: testIngressProof, OwnerVerified: true, FirewallReady: true}, testIngressProof},
		{"missing firewall", &PublicIngressObserved{Spec: spec, Proof: testIngressProof, OwnerVerified: true, ListenerReady: true}, testIngressProof},
	}
	for _, tt := range variants {
		t.Run(tt.name, func(t *testing.T) {
			next, e := TransitionVerifiedPublicIngressApplied(pending, tt.expected, tt.observed)
			if e == nil || next != pending {
				t.Fatalf("unsafe applied transition: %+v %v", next, e)
			}
		})
	}
	stale := *ready
	stale.Proof.Generation++
	if next, e := TransitionVerifiedPublicIngressApplied(pending, testIngressProof, &stale); e == nil || next != pending {
		t.Fatalf("stale proof accepted: %+v %v", next, e)
	}
	if next, e := TransitionVerifiedPublicIngressApplied(got, testIngressProof, ready); e == nil || next != got {
		t.Fatalf("duplicate applied transition accepted: %+v %v", next, e)
	}
}
