package ingressownership

import (
	"errors"
	"strings"
	"testing"
)

func TestNFTSnapshotFailsClosedOnUnverifiedIngress(t *testing.T) {
	cases := []string{"", `{}`, `{"nftables":[]}`, `{"nftables":[{"rule":{"family":"inet","table":"baft","chain":"ingress","handle":12,"comment":"baft-ingress-test","expr":[]}}]}`}
	for i, raw := range cases {
		if _, err := ParseNFTIngressRules([]byte(raw)); !errors.Is(err, ErrFirewallUnverified) {
			t.Fatalf("case %d accepted: %v", i, err)
		}
	}
	raw := `{"nftables":[{"metainfo":{"version":"1.0"}},{"rule":{"family":"inet","table":"unrelated","chain":"input","handle":10}}]}`
	observations, err := ParseNFTIngressRules([]byte(raw))
	if err != nil || len(observations) != 0 {
		t.Fatalf("unrelated rule: %+v %v", observations, err)
	}
}

func TestNFTSnapshotRejectsTrailingDocuments(t *testing.T) {
	valid := `{"nftables":[{"metainfo":{"version":"1.0"}}]}`
	for _, suffix := range []string{` {"nftables":[]}`, ` true`, ` []`, ` garbage`} {
		if _, err := ParseNFTIngressRules([]byte(valid + suffix)); !errors.Is(err, ErrFirewallUnverified) {
			t.Fatalf("trailing document accepted: %q %v", suffix, err)
		}
	}
}

func TestNFTExactTCPRuleCanBeReconciled(t *testing.T) {
	plan, err := PlanIngressRule(443, proof("nft", 3))
	if err != nil {
		t.Fatal(err)
	}
	raw := `{"nftables":[{"rule":{"family":"inet","table":"baft","chain":"ingress","handle":27,"comment":"` + plan.Tag + `","expr":[{"match":{"op":"==","left":{"payload":{"protocol":"tcp","field":"dport"}},"right":443}},{"accept":null}]}}]}`
	observed, err := ParseNFTIngressRules([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReconcileRuleObservation(plan, observed)
	if err != nil || got.Handle != 27 {
		t.Fatalf("exact rule not reconciled: %+v %v", got, err)
	}
	for _, broken := range []string{
		strings.Replace(raw, `"right":443`, `"right":22`, 1),
		strings.Replace(raw, `"field":"dport"`, `"field":"sport"`, 1),
		strings.Replace(raw, `{"accept":null}`, `{"drop":null}`, 1),
		strings.Replace(raw, `"handle":27`, `"handle":0`, 1),
	} {
		obs, parseErr := ParseNFTIngressRules([]byte(broken))
		if parseErr == nil {
			_, parseErr = ReconcileRuleObservation(plan, obs)
		}
		if !errors.Is(parseErr, ErrFirewallUnverified) {
			t.Fatalf("unsafe rule accepted: %v", parseErr)
		}
	}
}

func TestNFTRejectsInjectedMatchExpressions(t *testing.T) {
	plan, err := PlanIngressRule(443, proof("strict-nft", 5))
	if err != nil {
		t.Fatal(err)
	}
	base := `{"nftables":[{"rule":{"family":"inet","table":"baft","chain":"ingress","handle":27,"comment":"` + plan.Tag + `","expr":[{"match":{"op":"==","left":{"payload":{"protocol":"tcp","field":"dport"}},"right":443}},{"accept":null}]}}]}`
	for _, injected := range []string{
		strings.Replace(base, `"right":443`, `"right":443,"extra":true`, 1),
		strings.Replace(base, `"field":"dport"`, `"field":"dport","offset":1`, 1),
		strings.Replace(base, `"op":"=="`, `"op":"==","invert":true`, 1),
		strings.Replace(base, `"right":443`, `"right":443},"counter":{},"match":{"op":"==","left":{"payload":{"protocol":"tcp","field":"dport"}},"right":443`, 1),
	} {
		if _, err := ParseNFTIngressRules([]byte(injected)); !errors.Is(err, ErrFirewallUnverified) {
			t.Fatalf("injected expression accepted: %v", err)
		}
	}
}

func TestNFTRejectsOwnedTagOutsideManagedChain(t *testing.T) {
	plan, err := PlanIngressRule(54619, proof("spoofed-location", 1))
	if err != nil {
		t.Fatal(err)
	}
	for _, location := range []string{`"table":"unrelated","chain":"ingress"`, `"table":"baft","chain":"other"`, `"family":"ip","table":"baft","chain":"ingress"`} {
		raw := `{"nftables":[{"rule":{` + location + `,"handle":27,"comment":"` + plan.Tag + `"}}]}`
		if _, err := ParseNFTIngressRules([]byte(raw)); !errors.Is(err, ErrFirewallUnverified) {
			t.Fatalf("accepted owner tag outside managed chain: %s %v", location, err)
		}
	}
}
