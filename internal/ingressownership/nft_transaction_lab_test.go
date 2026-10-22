package ingressownership

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

type cancelAtPhaseJournal struct {
	MutationJournal
	phase  MutationPhase
	cancel context.CancelFunc
}

func (j cancelAtPhaseJournal) Advance(next MutationRecord) error {
	err := j.MutationJournal.Advance(next)
	if err == nil && next.Phase == j.phase {
		j.cancel()
	}
	return err
}

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

func TestNFTLabTransactionCancellationDoesNotAcknowledgeApplyOrRetire(t *testing.T) {
	spec := testSpec(t, "nft-cancel-tx")
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	applyCtx, cancelApply := context.WithCancel(context.Background())
	journal := new(LabJournal)
	fake := &nftLabFake{chainHandle: 7}
	tx := &NFTLabTransaction{Runtime: NewRuntime(), Firewall: &NFTLabBackend{Runner: fake, ExpectedChainHandle: 7}, Journal: cancelAtPhaseJournal{MutationJournal: journal, phase: PhaseVerified, cancel: cancelApply}}
	if err := tx.Apply(applyCtx, spec, proof); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled apply acknowledged: %v", err)
	}
	if fake.ruleHandle != 0 {
		t.Fatal("cancelled apply left firewall rule")
	}
	ln, err := net.Listen("tcp", spec.Config.Listen)
	if err != nil {
		t.Fatalf("cancelled apply left socket: %v", err)
	}
	ln.Close()

	// A separate clean transaction verifies retirement is fenced before the
	// exact-owner socket or firewall is removed.
	spec = testSpec(t, "nft-cancel-retire")
	proof, err = ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	journal = new(LabJournal)
	fake = &nftLabFake{chainHandle: 7}
	tx = &NFTLabTransaction{Runtime: NewRuntime(), Firewall: &NFTLabBackend{Runner: fake, ExpectedChainHandle: 7}, Journal: journal}
	if err := tx.Apply(context.Background(), spec, proof); err != nil {
		t.Fatal(err)
	}
	retireCtx, cancelRetire := context.WithCancel(context.Background())
	tx.Journal = cancelAtPhaseJournal{MutationJournal: journal, phase: PhaseRetireIntent, cancel: cancelRetire}
	if err := tx.Retire(retireCtx, spec, proof); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled retirement continued: %v", err)
	}
	if fake.ruleHandle == 0 {
		t.Fatal("cancelled retirement removed firewall rule")
	}
	tx.Journal = journal
	if err := tx.Retire(context.Background(), spec, proof); err != nil {
		t.Fatalf("safe retirement retry failed: %v", err)
	}
}
