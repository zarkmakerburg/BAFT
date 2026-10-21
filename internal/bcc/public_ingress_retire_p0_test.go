package bcc

import "testing"

func TestP0IngressRetireProof(t *testing.T) {
	spec, err := PlanPublicIngress("arno", "tun-a", 54619, "127.0.0.1:1444")
	if err != nil {
		t.Fatal(err)
	}
	proof := PublicIngressRetireProof{Spec: spec, Proof: testIngressProof, OwnerVerified: true}
	if err := CanRetirePublicIngress(spec, testIngressProof, proof); err != nil {
		t.Fatal(err)
	}
	stale := testIngressProof
	stale.Generation--
	if err := CanRetirePublicIngress(spec, stale, proof); err == nil {
		t.Fatal("stale generation must fail")
	}
	stale = testIngressProof
	stale.ManifestDigest = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if err := CanRetirePublicIngress(spec, stale, proof); err == nil {
		t.Fatal("different digest must fail")
	}
	stale = testIngressProof
	stale.FirewallID = "another-rule"
	if err := CanRetirePublicIngress(spec, stale, proof); err == nil {
		t.Fatal("different firewall identity must fail")
	}
	stale = testIngressProof
	stale.Generation = 0
	if err := CanRetirePublicIngress(spec, stale, proof); err == nil {
		t.Fatal("zero generation must fail")
	}
	proof.OwnerVerified = false
	if err := CanRetirePublicIngress(spec, testIngressProof, proof); err == nil {
		t.Fatal("unverified ownership must fail")
	}
	proof.OwnerVerified = true
	proof.Spec.TunnelID = "tun-b"
	if err := CanRetirePublicIngress(spec, testIngressProof, proof); err == nil {
		t.Fatal("other tunnel resource must not be retired")
	}
}

func TestP0IngressRetireCanonicalAndMalformedProof(t *testing.T) {
	desired, err := PlanPublicIngress("ir-main", "tun-a", 2053, "127.0.0.1:9000")
	if err != nil {
		t.Fatal(err)
	}
	proof := PublicIngressRetireProof{
		Spec:  PublicIngressSpec{IRNode: " ir-main ", TunnelID: " tun-a ", PublicPort: 2053, LocalTarget: "127.0.0.1:09000"},
		Proof: testIngressProof, OwnerVerified: true,
	}
	if err := CanRetirePublicIngress(desired, testIngressProof, proof); err != nil {
		t.Fatalf("canonical proof rejected: %v", err)
	}
	bad := proof
	bad.Spec.LocalTarget = "0.0.0.0:9000"
	if err := CanRetirePublicIngress(desired, testIngressProof, bad); err == nil {
		t.Fatal("malformed proof accepted")
	}
	bad = proof
	bad.Spec.PublicPort = 2054
	if err := CanRetirePublicIngress(desired, testIngressProof, bad); err == nil {
		t.Fatal("different port proof accepted")
	}
	bad = proof
	bad.Spec.TunnelID = "tun-b"
	if err := CanRetirePublicIngress(desired, testIngressProof, bad); err == nil {
		t.Fatal("different owner proof accepted")
	}
}
