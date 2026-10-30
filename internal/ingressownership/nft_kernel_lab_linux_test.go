//go:build linux

package ingressownership

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

func unrelatedRules(raw []byte) ([]json.RawMessage, error) {
	var root struct {
		NFTables []json.RawMessage `json:"nftables"`
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	var rules []json.RawMessage
	for _, item := range root.NFTables {
		var wrapper map[string]json.RawMessage
		if err := json.Unmarshal(item, &wrapper); err != nil {
			return nil, err
		}
		var rule struct {
			Family string `json:"family"`
			Table  string `json:"table"`
			Chain  string `json:"chain"`
		}
		if err := json.Unmarshal(wrapper["rule"], &rule); err == nil && rule.Family == "inet" && rule.Table == "unrelated" && rule.Chain == "keep" {
			rules = append(rules, wrapper["rule"])
		}
	}
	return rules, nil
}

func TestNFTKernelLAB(t *testing.T) {
	if os.Getenv("BAFT_NFT_KERNEL_LAB") != "1" {
		t.Skip("run only in the dedicated CI network namespace")
	}
	self, err := os.Readlink("/proc/self/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	init, err := os.Readlink("/proc/1/ns/net")
	if err != nil || self == init || os.Geteuid() != 0 {
		t.Fatal("nft kernel LAB requires root inside a separate network namespace")
	}
	const binary = "/usr/sbin/nft"
	if _, err := os.Stat(binary); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	run := func(args ...string) {
		t.Helper()
		output, err := exec.CommandContext(ctx, binary, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("nft %v: %v: %s", args, err, output)
		}
	}
	run("add", "table", "inet", "baft")
	defer exec.Command(binary, "delete", "table", "inet", "baft").Run()
	run("add", "chain", "inet", "baft", "ingress", "{ type filter hook input priority -100; policy accept; }")
	run("add", "table", "inet", "unrelated")
	defer exec.Command(binary, "delete", "table", "inet", "unrelated").Run()
	run("add", "chain", "inet", "unrelated", "keep")
	run("add", "rule", "inet", "unrelated", "keep", "tcp", "dport", "2053", "accept")
	raw, err := (NFTCommandRunner{Binary: binary}).ReadRuleset(ctx)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := unrelatedRules(raw)
	if err != nil || len(baseline) != 1 {
		t.Fatalf("unrelated baseline missing: %v", err)
	}
	var snapshot struct {
		NFTables []struct {
			Chain struct {
				Family string `json:"family"`
				Table  string `json:"table"`
				Name   string `json:"name"`
				Handle uint64 `json:"handle"`
			} `json:"chain"`
		} `json:"nftables"`
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	var chainHandle uint64
	for _, element := range snapshot.NFTables {
		c := element.Chain
		if c.Family == "inet" && c.Table == "baft" && c.Name == "ingress" {
			chainHandle = c.Handle
		}
	}
	if chainHandle == 0 {
		t.Fatal("kernel chain handle unavailable")
	}
	plan, err := PlanIngressRule(54619, proof("kernel-lab", 1))
	if err != nil {
		t.Fatal(err)
	}
	backend := &NFTLabBackend{Runner: NFTCommandRunner{Binary: binary}, ExpectedChainHandle: chainHandle}
	observed, err := backend.Install(ctx, plan)
	if err != nil || observed.Handle == 0 {
		t.Fatalf("kernel rule install: %+v %v", observed, err)
	}
	if _, err := backend.Install(ctx, plan); err != nil {
		t.Fatalf("kernel idempotent install: %v", err)
	}
	if err := backend.Remove(ctx, plan, observed.Handle+1); !errors.Is(err, ErrFirewallUnverified) {
		t.Fatalf("wrong kernel handle accepted: %v", err)
	}
	if err := backend.Remove(ctx, plan, observed.Handle); err != nil {
		t.Fatalf("kernel rule removal: %v", err)
	}
	if _, err := backend.Install(ctx, plan); err != nil {
		t.Fatalf("kernel reinstall: %v", err)
	}
	raw, err = (NFTCommandRunner{Binary: binary}).ReadRuleset(ctx)
	if err != nil || !bytes.Contains(raw, []byte(plan.Tag)) {
		t.Fatalf("reinstalled kernel rule missing: %v", err)
	}
	last, err := backend.Inspect(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.Remove(ctx, plan, last.Handle); err != nil {
		t.Fatal(err)
	}
	journal, err := OpenDurableJournal(newJournalTestDir(t))
	if err != nil {
		t.Fatal(err)
	}
	spec := ingressmanifest.Spec{TunnelID: "kernel-transaction", InstanceID: "lab-ir", Generation: 1, Config: ingressmanifest.Config{
		Listen: "127.0.0.1:54619", Target: "127.0.0.1:1443", MaxClients: 16, DialTimeoutMS: 3000,
	}}
	owner, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	tx := &NFTLabTransaction{Runtime: NewRuntime(), Firewall: backend, Journal: journal}
	if err := tx.Apply(ctx, spec, owner); err != nil {
		t.Fatalf("journaled kernel apply: %v", err)
	}
	if err := tx.Retire(ctx, spec, owner); err != nil {
		t.Fatalf("journaled kernel retire: %v", err)
	}
	recovery, err := journal.Recover()
	if err != nil || len(recovery.NeedsReconciliation) != 0 || len(recovery.Records) != 1 || recovery.Records[0].Phase != PhaseRetired {
		t.Fatalf("kernel transaction journal: %+v %v", recovery, err)
	}
	raw, err = (NFTCommandRunner{Binary: binary}).ReadRuleset(ctx)
	if err != nil {
		t.Fatal(err)
	}
	after, err := unrelatedRules(raw)
	if err != nil || len(after) != 1 || !bytes.Equal(baseline[0], after[0]) {
		t.Fatal("unrelated 2053 rule changed during BAFT rule lifecycle")
	}
}
