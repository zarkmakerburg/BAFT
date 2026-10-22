package ingressownership

import (
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
