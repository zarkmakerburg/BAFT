package ingressownership

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

func TestNFTLabTransactionJournaledLifecycle(t *testing.T) {
	j := new(LabJournal)
	fake := &nftLabFake{chainHandle: 7}
	tx := &NFTLabTransaction{Runtime: NewRuntime(), Firewall: &NFTLabBackend{Runner: fake, ExpectedChainHandle: 7}, Journal: j}
	spec := testSpec(t, "nft-transaction")
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Apply(context.Background(), spec, proof); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := j.Current(proof); err != nil || !ok || got.Phase != PhaseVerified {
		t.Fatalf("verified journal: %+v %t %v", got, ok, err)
	}
	if err := tx.Apply(context.Background(), spec, proof); err != nil {
		t.Fatalf("idempotent apply: %v", err)
	}
	if len(fake.commands) != 1 {
		t.Fatalf("duplicate nft mutation: %v", fake.commands)
	}
	if err := tx.Retire(context.Background(), spec, proof); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := j.Current(proof); err != nil || !ok || got.Phase != PhaseRetired {
		t.Fatalf("retired journal: %+v %t %v", got, ok, err)
	}
	if len(fake.commands) != 2 {
		t.Fatalf("wrong nft command count: %v", fake.commands)
	}
}

func TestNFTLabTransactionRefusesMissingJournalOrKernelProof(t *testing.T) {
	fake := &nftLabFake{chainHandle: 7}
	spec := testSpec(t, "nft-no-journal")
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	tx := &NFTLabTransaction{Runtime: NewRuntime(), Firewall: &NFTLabBackend{Runner: fake, ExpectedChainHandle: 7}}
	if err := tx.Apply(context.Background(), spec, proof); !errors.Is(err, ErrInvalid) {
		t.Fatalf("applied without journal: %v", err)
	}
	ln, err := net.Listen("tcp", spec.Config.Listen)
	if err != nil {
		t.Fatalf("listener mutated without journal: %v", err)
	}
	ln.Close()
	if len(fake.commands) != 0 {
		t.Fatal("firewall mutated without journal")
	}
	tx.Journal = new(LabJournal)
	if err := tx.Apply(context.Background(), spec, proof); err != nil {
		t.Fatal(err)
	}
	fake.ruleHandle++
	if err := tx.Retire(context.Background(), spec, proof); !errors.Is(err, ErrFirewallUnverified) {
		t.Fatalf("accepted swapped kernel handle: %v", err)
	}
	fake.ruleHandle--
	if err := tx.Retire(context.Background(), spec, proof); err != nil {
		t.Fatalf("could not retire restored exact owner: %v", err)
	}
}
