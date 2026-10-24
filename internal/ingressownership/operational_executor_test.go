package ingressownership

import (
	"context"
	"errors"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
)

// Even a structurally complete signed CommandV1 is NOT a live lease.
// Never touch a listener, journal, or firewall when the concrete Alpha
// capability cannot independently validate its OS-held operation lock.
func TestOperationalExecutorNoMutationWithoutConcreteAlphaLease(t *testing.T) {
	cmd := executorCommandV1(t)
	journal := new(LabJournal)
	rt := NewRuntime()
	nft := &NFTLabBackend{Runner: &nftLabFake{chainHandle: 7}, ExpectedChainHandle: 7}
	x, err := NewOperationalExecutor(rt, nft, journal)
	if err != nil {
		t.Fatal(err)
	}
	_, err = x.Apply(context.Background(), cmd, nil)
	if !errors.Is(err, ErrExecutorTrustUnavailable) {
		t.Fatalf("nil lease accepted by Apply: %v", err)
	}
	var fakeZero ingressauthority.Lease
	if out, err := x.Apply(context.Background(), cmd, &fakeZero); err == nil || out.Listener.SocketIdentity != "" {
		t.Fatalf("zero/JSON-made permit allowed Apply: evidence=%+v err=%v", out, err)
	}
	retire := cmd
	retire.JobID = "new-retire-command"
	retire.Action = "public_ingress_retire"
	if err := retire.Validate(retire.IssuedAt.Add(1)); err != nil {
		t.Fatal(err)
	}
	if _, err := x.Retire(context.Background(), retire, &fakeZero); !errors.Is(err, ingressauthority.ErrUnverified) {
		t.Fatalf("different signed retire job bypassed concrete lease: %v", err)
	}
	if len(rt.sessions) != 0 {
		t.Fatal("executor bound a socket without a live Alpha lease")
	}
	if entries := journal.Snapshot(); len(entries) != 0 {
		t.Fatalf("executor wrote durable journal without a lease: %+v", entries)
	}
	if nft.Runner.(*nftLabFake).ruleHandle != 0 {
		t.Fatal("executor opened firewall without a live lease")
	}
	if _, err := NewOperationalExecutor(rt, nil, journal); err == nil {
		t.Fatal("nil nft backend accepted")
	}
	if _, err := NewOperationalExecutor(rt, nft, nil); err == nil {
		t.Fatal("nil durable journal accepted")
	}
}

// Operational nft transaction methods accept only Alpha's concrete lease:
// the older LAB methods remain separate and cannot accidentally be called
// as the signed operational executor's effectful path.
func TestGuardedNFTTransactionFencesEveryMutationBeforeFirstEffect(t *testing.T) {
	cmd := executorCommandV1(t)
	journal := new(LabJournal)
	rt := NewRuntime()
	fake := &nftLabFake{chainHandle: 7}
	tx := &NFTLabTransaction{Runtime: rt, Firewall: &NFTLabBackend{
		Runner: fake, ExpectedChainHandle: 7,
	}, Journal: journal}
	var unminted ingressauthority.Lease
	if err := tx.ApplyWithLease(context.Background(), cmd.Binding.Spec, cmd.Binding.Proof, cmd, &unminted); !errors.Is(err, ingressauthority.ErrUnverified) {
		t.Fatalf("unminted concrete lease reached effectful Apply: %v", err)
	}
	retire := cmd
	retire.JobID = "independent-retire"
	retire.Action = "public_ingress_retire"
	if err := tx.RetireWithLease(context.Background(), retire.Binding.Spec, retire.Binding.Proof, retire, &unminted); !errors.Is(err, ingressauthority.ErrUnverified) {
		t.Fatalf("unminted concrete lease reached effectful Retire: %v", err)
	}
	if len(rt.sessions) != 0 || len(journal.Snapshot()) != 0 || fake.ruleHandle != 0 {
		t.Fatal("operational transaction changed listener/journal/nft without held Alpha lease")
	}
	if err := tx.ApplyWithLease(context.Background(), cmd.Binding.Spec, cmd.Binding.Proof, cmd, nil); !errors.Is(err, ErrExecutorTrustUnavailable) {
		t.Fatalf("nil live lease reached effectful transaction: %v", err)
	}
}
