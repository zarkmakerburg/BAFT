package ingressownership

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

type failRetiredJournal struct {
	MutationJournal
	fail bool
}

var errFinalizeFault = errors.New("LAB final journal write failed")

func (j *failRetiredJournal) Advance(r MutationRecord) error {
	if j.fail && r.Phase == PhaseRetired {
		return errFinalizeFault
	}
	return j.MutationJournal.Advance(r)
}

// A successful resource deletion must not become permanently unfinishable
// when the final durable write fails. A new process cannot inherit that proof.
func TestNFTLabRetireFinalizesAfterJournalFailure(t *testing.T) {
	spec := testSpec(t, "retire-finalize")
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	fake := &nftLabFake{chainHandle: 7}
	journal := &failRetiredJournal{MutationJournal: new(LabJournal)}
	tx := &NFTLabTransaction{Runtime: NewRuntime(), Firewall: &NFTLabBackend{Runner: fake, ExpectedChainHandle: 7}, Journal: journal}
	if err := tx.Apply(context.Background(), spec, proof); err != nil {
		t.Fatal(err)
	}
	defer tx.Runtime.Retire(spec.Config.Listen, proof)
	journal.fail = true
	if err := tx.Retire(context.Background(), spec, proof); !errors.Is(err, errFinalizeFault) {
		t.Fatalf("lost final write failure: %v", err)
	}
	current, _, err := journal.Current(proof)
	if err != nil || current.Phase != PhaseRetireIntent || fake.ruleHandle != 0 {
		t.Fatalf("wrong interrupted state: %+v %v", current, err)
	}
	foreign, err := listenWithForensics(t, "tcp4", spec.Config.Listen)
	if err != nil {
		t.Fatal(err)
	}
	defer foreign.Close()
	journal.fail = false
	commands := len(fake.commands)
	restarted := &NFTLabTransaction{Runtime: NewRuntime(), Firewall: tx.Firewall, Journal: journal}
	if err := restarted.Retire(context.Background(), spec, proof); err == nil {
		t.Fatal("restart adopted deletion proof from journal")
	}
	if err := tx.Retire(context.Background(), spec, proof); err != nil {
		t.Fatalf("cannot finalize independently verified deletion: %v", err)
	}
	current, _, err = journal.Current(proof)
	if err != nil || current.Phase != PhaseRetired || len(fake.commands) != commands {
		t.Fatalf("finalization mutated resources or failed: %+v %v", current, err)
	}
	c, err := net.Dial("tcp4", foreign.Addr().String())
	if err != nil {
		t.Fatalf("finalization stopped replacement listener: %v", err)
	}
	_ = c.Close()
}

func TestNFTLabRetireFinalizationRechecksResources(t *testing.T) {
	for _, fault := range []string{"read", "chain", "chain-pin", "rule", "runtime", "backend", "cancel"} {
		t.Run(fault, func(t *testing.T) {
			spec := testSpec(t, "finalize-fence")
			proof, err := ingressmanifest.Preview(spec)
			if err != nil {
				t.Fatal(err)
			}
			fake := &nftLabFake{chainHandle: 7}
			journal := &failRetiredJournal{MutationJournal: new(LabJournal)}
			runtime := NewRuntime()
			backend := &NFTLabBackend{Runner: fake, ExpectedChainHandle: 7}
			tx := &NFTLabTransaction{Runtime: runtime, Firewall: backend, Journal: journal}
			if err := tx.Apply(context.Background(), spec, proof); err != nil {
				t.Fatal(err)
			}
			defer runtime.Retire(spec.Config.Listen, proof)
			journal.fail = true
			if err := tx.Retire(context.Background(), spec, proof); !errors.Is(err, errFinalizeFault) {
				t.Fatal(err)
			}
			journal.fail = false
			commands := len(fake.commands)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch fault {
			case "read":
				fake.failRead = true
			case "chain":
				fake.chainHandle++
			case "chain-pin":
				fake.chainHandle++
				backend.ExpectedChainHandle++
			case "rule":
				fake.ruleHandle = 43
			case "runtime":
				tx.Runtime = NewRuntime()
			case "backend":
				tx.Firewall = &NFTLabBackend{Runner: fake, ExpectedChainHandle: 7}
			case "cancel":
				cancel()
			}
			if err := tx.Retire(ctx, spec, proof); err == nil {
				t.Fatal("ambiguous finalization claimed success")
			}
			current, _, err := journal.Current(proof)
			if err != nil || current.Phase != PhaseRetireIntent || len(fake.commands) != commands {
				t.Fatalf("failed check changed state: %+v %v", current, err)
			}
			fake.failRead, fake.chainHandle, fake.ruleHandle = false, 7, 0
			backend.ExpectedChainHandle = 7
			tx.Runtime, tx.Firewall = runtime, backend
			if err := tx.Retire(context.Background(), spec, proof); err != nil {
				t.Fatalf("could not finish after restored proof: %v", err)
			}
		})
	}
}

func TestNFTLabRetireUncertainDeleteCannotBecomeFinalizationProof(t *testing.T) {
	spec := testSpec(t, "finalize-uncertain")
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	fake := &nftLabFake{chainHandle: 7}
	journal := new(LabJournal)
	tx := &NFTLabTransaction{Runtime: NewRuntime(), Firewall: &NFTLabBackend{Runner: fake, ExpectedChainHandle: 7}, Journal: journal}
	if err := tx.Apply(context.Background(), spec, proof); err != nil {
		t.Fatal(err)
	}
	defer tx.Runtime.Retire(spec.Config.Listen, proof)
	fake.partialDel = true
	if err := tx.Retire(context.Background(), spec, proof); err == nil {
		t.Fatal("uncertain deletion claimed success")
	}
	if fake.ruleHandle != 0 {
		t.Fatal("fault did not remove rule")
	}
	fake.partialDel = false
	commands := len(fake.commands)
	if err := tx.Retire(context.Background(), spec, proof); err == nil {
		t.Fatal("absence adopted as successful deletion")
	}
	current, _, err := journal.Current(proof)
	if err != nil || current.Phase != PhaseRetireIntent || len(fake.commands) != commands {
		t.Fatalf("uncertain delete changed state: %+v %v", current, err)
	}
}
