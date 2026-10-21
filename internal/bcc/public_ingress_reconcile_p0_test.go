package bcc

import "testing"

var testIngressProof = PublicIngressExpectedProof{Generation: 7, ManifestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ListenerID: "baft-listener-7", FirewallID: "baft-firewall-7"}

func TestP0IngressReconcileDecision(t *testing.T) {
	spec, err := PlanPublicIngress("arno", "tun-a", 54619, "127.0.0.1:1444")
	if err != nil {
		t.Fatal(err)
	}
	check := func(obs *PublicIngressObserved, want PublicIngressReconcileDecision, wantErr bool) {
		t.Helper()
		got, err := DecidePublicIngressReconcile(spec, testIngressProof, obs)
		if got != want || (err != nil) != wantErr {
			t.Fatalf("got %s, %v; want %s error=%v", got, err, want, wantErr)
		}
	}
	check(nil, PublicIngressNeedsApply, false)
	if got, err := DecidePublicIngressReconcile(spec, PublicIngressExpectedProof{}, nil); got != PublicIngressBlocked || err == nil {
		t.Fatalf("missing durable proof accepted: %s, %v", got, err)
	}
	check(&PublicIngressObserved{Spec: spec, Proof: testIngressProof, OwnerVerified: true, ListenerReady: true, FirewallReady: true}, PublicIngressNoop, false)
	check(&PublicIngressObserved{Spec: spec, Proof: testIngressProof, OwnerVerified: true, ListenerReady: false, FirewallReady: true}, PublicIngressNeedsApply, false)
	check(&PublicIngressObserved{Spec: spec, Proof: testIngressProof, OwnerVerified: false, ListenerReady: true, FirewallReady: true}, PublicIngressBlocked, true)
	stale := testIngressProof
	stale.Generation--
	check(&PublicIngressObserved{Spec: spec, Proof: stale, OwnerVerified: true, ListenerReady: true, FirewallReady: true}, PublicIngressBlocked, true)
	stale = testIngressProof
	stale.ManifestDigest = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	check(&PublicIngressObserved{Spec: spec, Proof: stale, OwnerVerified: true, ListenerReady: true, FirewallReady: true}, PublicIngressBlocked, true)
	stale = testIngressProof
	stale.ListenerID = "another-listener"
	check(&PublicIngressObserved{Spec: spec, Proof: stale, OwnerVerified: true, ListenerReady: true, FirewallReady: true}, PublicIngressBlocked, true)
	mismatch := spec
	mismatch.LocalTarget = "127.0.0.1:1445"
	check(&PublicIngressObserved{Spec: mismatch, Proof: testIngressProof, OwnerVerified: true}, PublicIngressBlocked, true)
	mismatch = spec
	mismatch.PublicPort = 2453
	check(&PublicIngressObserved{Spec: mismatch, Proof: testIngressProof, OwnerVerified: true}, PublicIngressBlocked, true)
}

func TestP0IngressReconcileCanonicalObservedAndMalformed(t *testing.T) {
	desired, err := PlanPublicIngress("ir-main", "tun-a", 2453, "127.0.0.1:9000")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name                      string
		spec                      PublicIngressSpec
		owner, listener, firewall bool
		want                      PublicIngressReconcileDecision
		wantError                 bool
	}{
		{"canonical IPv4 port", PublicIngressSpec{IRNode: " ir-main ", TunnelID: " tun-a ", PublicPort: 2453, LocalTarget: "127.0.0.1:09000"}, true, true, true, PublicIngressNoop, false},
		{"canonical IPv4 mapped", PublicIngressSpec{IRNode: "ir-main", TunnelID: "tun-a", PublicPort: 2453, LocalTarget: "[::ffff:127.0.0.1]:9000"}, true, true, true, PublicIngressNoop, false},
		{"partial listener", desired, true, false, true, PublicIngressNeedsApply, false},
		{"partial firewall", desired, true, true, false, PublicIngressNeedsApply, false},
		{"unverified owner", desired, false, true, true, PublicIngressBlocked, true},
		{"malformed observed", PublicIngressSpec{IRNode: "ir-main", TunnelID: "tun-a", PublicPort: 2453, LocalTarget: "0.0.0.0:9000"}, true, true, true, PublicIngressBlocked, true},
		{"different tunnel", PublicIngressSpec{IRNode: "ir-main", TunnelID: "tun-b", PublicPort: 2453, LocalTarget: "127.0.0.1:9000"}, true, true, true, PublicIngressBlocked, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obs := &PublicIngressObserved{Spec: tt.spec, Proof: testIngressProof, OwnerVerified: tt.owner, ListenerReady: tt.listener, FirewallReady: tt.firewall}
			got, err := DecidePublicIngressReconcile(desired, testIngressProof, obs)
			if got != tt.want || (err != nil) != tt.wantError {
				t.Fatalf("decision=%s err=%v want=%s error=%t", got, err, tt.want, tt.wantError)
			}
		})
	}
}
