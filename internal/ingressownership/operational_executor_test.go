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
