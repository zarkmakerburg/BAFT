package ingressownership

import (
	"context"
	"errors"
	"strconv"
	"testing"
)

func TestNFTMutationCommandAllowlist(t *testing.T) {
	plan, _ := PlanIngressRule(443, proof("command-allowlist", 1))
	good := [][]string{{"add", "rule", "inet", "baft", "ingress", "tcp", "dport", "443", "accept", "comment", plan.Tag}, {"delete", "rule", "inet", "baft", "ingress", "handle", "42"}}
	for _, args := range good {
		if !allowedNFTMutation(args) {
			t.Fatalf("rejected legitimate command: %v", args)
		}
	}
	bad := [][]string{{"flush", "ruleset"}, {"delete", "table", "inet", "baft"}, {"add", "rule", "inet", "baft", "ingress", "tcp", "dport", "22", "accept", "comment", plan.Tag}, {"add", "rule", "inet", "baft", "ingress", "tcp", "dport", "443", "accept", "comment", "bad"}, {"delete", "rule", "inet", "baft", "ingress", "handle", "0"}, {"delete", "rule", "inet", "baft", "ingress", "handle", "42", "extra"}}
	for _, args := range bad {
		if allowedNFTMutation(args) {
			t.Fatalf("allowed unsafe command: %v", args)
		}
	}
	if !validNFTTag(plan.Tag) || validNFTTag("baft-ingress-"+strconv.FormatUint(1, 16)) {
		t.Fatal("invalid ownership tag validation")
	}
}

func TestNFTMutationRejectsNoncanonicalArguments(t *testing.T) {
	plan, err := PlanIngressRule(443, proof("nft-canonical", 1))
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"add", "rule", "inet", "baft", "ingress", "tcp", "dport", "0443", "accept", "comment", plan.Tag},
		{"delete", "rule", "inet", "baft", "ingress", "handle", "042"},
		{"delete", "rule", "inet", "baft", "ingress", "handle", "42", ""},
	} {
		if allowedNFTMutation(args) {
			t.Fatalf("accepted noncanonical command: %v", args)
		}
	}
}

func TestNFTCommandRunnerRejectsNilContext(t *testing.T) {
	plan, err := PlanIngressRule(443, proof("nft-nil-run", 1))
	if err != nil {
		t.Fatal(err)
	}
	runner := NFTCommandRunner{Binary: "/usr/sbin/nft"}
	if err := runner.Run(nil, "add", "rule", "inet", "baft", "ingress", "tcp", "dport", "443", "accept", "comment", plan.Tag); !errors.Is(err, ErrFirewallUnverified) {
		t.Fatalf("nil context accepted: %v", err)
	}
	if _, err := (&NFTLabBackend{Runner: &nftLabFake{chainHandle: 7}, ExpectedChainHandle: 7}).inspect(nil); !errors.Is(err, ErrFirewallUnverified) {
		t.Fatalf("nil inspection accepted: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runner.Run(ctx, "add", "rule", "inet", "baft", "ingress", "tcp", "dport", "443", "accept", "comment", plan.Tag); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context accepted: %v", err)
	}
}
