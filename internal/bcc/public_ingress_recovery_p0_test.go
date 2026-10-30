package bcc

import "testing"

func TestP0PublicIngressRecoveryFailClosed(t *testing.T) {
	spec, err := PlanPublicIngress("arno", "tun-a", 54619, "127.0.0.1:1444")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		obs  *PublicIngressObserved
		want PublicIngressRecoveryDecision
	}{
		{"no observation", nil, IngressRecoveryBlocked},
		{"listener only", &PublicIngressObserved{Spec: spec, Proof: testIngressProof, OwnerVerified: true, ListenerReady: true}, IngressRecoveryInspect},
		{"firewall only", &PublicIngressObserved{Spec: spec, Proof: testIngressProof, OwnerVerified: true, FirewallReady: true}, IngressRecoveryInspect},
		{"both ready", &PublicIngressObserved{Spec: spec, Proof: testIngressProof, OwnerVerified: true, ListenerReady: true, FirewallReady: true}, IngressRecoveryNoop},
		{"neither ready", &PublicIngressObserved{Spec: spec, Proof: testIngressProof, OwnerVerified: true}, IngressRecoveryBlocked},
		{"unknown owner", &PublicIngressObserved{Spec: spec, Proof: testIngressProof, ListenerReady: true}, IngressRecoveryBlocked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := PlanPublicIngressRecovery(spec, testIngressProof, tc.obs)
			if got != tc.want {
				t.Fatalf("decision=%s want=%s", got, tc.want)
			}
		})
	}
	stale := &PublicIngressObserved{Spec: spec, Proof: testIngressProof, OwnerVerified: true, ListenerReady: true}
	stale.Proof.Generation++
	if got, _ := PlanPublicIngressRecovery(spec, testIngressProof, stale); got != IngressRecoveryBlocked {
		t.Fatal("stale generation allowed recovery")
	}
}

func TestP0PublicIngressRecoveryRejectsMismatchedPartialResources(t *testing.T) {
	spec, err := PlanPublicIngress("arno", "tun-a", 54619, "127.0.0.1:1444")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*PublicIngressObserved)
	}{
		{"wrong tunnel", func(o *PublicIngressObserved) { o.Spec.TunnelID = "tun-other" }},
		{"wrong port", func(o *PublicIngressObserved) { o.Spec.PublicPort++ }},
		{"wrong target", func(o *PublicIngressObserved) { o.Spec.LocalTarget = "127.0.0.1:1445" }},
		{"wrong node", func(o *PublicIngressObserved) { o.Spec.IRNode = "other-node" }},
		{"wrong digest", func(o *PublicIngressObserved) {
			o.Proof.ManifestDigest = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		}},
		{"wrong firewall identity", func(o *PublicIngressObserved) { o.Proof.FirewallID = "other-firewall" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			observed := &PublicIngressObserved{Spec: spec, Proof: testIngressProof, OwnerVerified: true, ListenerReady: true}
			tc.change(observed)
			got, err := PlanPublicIngressRecovery(spec, testIngressProof, observed)
			if got != IngressRecoveryBlocked || err == nil {
				t.Fatalf("mismatched resource accepted: %s, %v", got, err)
			}
		})
	}
}
