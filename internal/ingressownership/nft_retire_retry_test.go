package ingressownership

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

func TestNFTLabRetireRetriesAfterOwnedSocketStopped(t *testing.T) {
	spec := testSpec(t, "retire-retry")
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	fake := &nftLabFake{chainHandle: 7}
	journal := new(LabJournal)
	backend := &NFTLabBackend{Runner: fake, ExpectedChainHandle: 7}
	tx := &NFTLabTransaction{Runtime: NewRuntime(), Firewall: backend, Journal: journal}
	if err := tx.Apply(context.Background(), spec, proof); err != nil {
		t.Fatal(err)
	}
	defer tx.Runtime.Retire(spec.Config.Listen, proof)
	fake.failCommand = true
	if err := tx.Retire(context.Background(), spec, proof); err == nil {
		t.Fatal("failed delete claimed success")
	}
	current, found, err := journal.Current(proof)
	if err != nil || !found || current.Phase != PhaseRetireIntent || fake.ruleHandle == 0 {
		t.Fatalf("lost retirement uncertainty: %+v %v", current, err)
	}
	// Rebinding proves the original listener is gone. The retry must not
	// stop this unrelated service just because it now occupies the old port.
	foreign, err := net.Listen("tcp4", spec.Config.Listen)
	if err != nil {
		t.Fatalf("owned listener did not stop: %v", err)
	}
	defer foreign.Close()
	fake.failCommand = false
	if err := tx.Retire(context.Background(), spec, proof); err != nil {
		t.Fatalf("owned retirement retry: %v", err)
	}
	if fake.ruleHandle != 0 {
		t.Fatal("retry left exact-owned firewall rule")
	}
	current, _, err = journal.Current(proof)
	if err != nil || current.Phase != PhaseRetired {
		t.Fatalf("retirement not persisted: %+v %v", current, err)
	}
	c, err := net.Dial("tcp4", foreign.Addr().String())
	if err != nil {
		t.Fatalf("retry stopped unrelated listener: %v", err)
	}
	_ = c.Close()
}

func TestNFTLabRetireRestartCannotUseJournalAsStoppedSocketProof(t *testing.T) {
	spec := testSpec(t, "retire-restart")
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	fake := &nftLabFake{chainHandle: 7}
	journal := new(LabJournal)
	backend := &NFTLabBackend{Runner: fake, ExpectedChainHandle: 7}
	tx := &NFTLabTransaction{Runtime: NewRuntime(), Firewall: backend, Journal: journal}
	if err := tx.Apply(context.Background(), spec, proof); err != nil {
		t.Fatal(err)
	}
	defer tx.Runtime.Retire(spec.Config.Listen, proof)
	fake.failCommand = true
	if err := tx.Retire(context.Background(), spec, proof); err == nil {
		t.Fatal("failed deletion claimed success")
	}
	fake.failCommand = false
	commands := len(fake.commands)
	restarted := &NFTLabTransaction{Runtime: NewRuntime(), Firewall: backend, Journal: journal}
	if err := restarted.Retire(context.Background(), spec, proof); !errors.Is(err, ErrUnknown) {
		t.Fatalf("restart adopted journal proof: %v", err)
	}
	if len(fake.commands) != commands || fake.ruleHandle == 0 {
		t.Fatal("restart changed firewall")
	}
	// Changed live ownership must still fence an in-process retry.
	fake.ruleHandle++
	if err := tx.Retire(context.Background(), spec, proof); !errors.Is(err, ErrFirewallUnverified) {
		t.Fatalf("retry accepted replaced rule: %v", err)
	}
	if len(fake.commands) != commands {
		t.Fatal("retry deleted replaced rule")
	}
	fake.ruleHandle--
	if err := tx.Retire(context.Background(), spec, proof); err != nil {
		t.Fatalf("exact owner could not finish: %v", err)
	}
}

func TestNFTLabRetireRetryFencesChangedExecutor(t *testing.T) {
	for _, change := range []string{"runtime", "firewall"} {
		t.Run(change, func(t *testing.T) {
			spec := testSpec(t, "retire-executor")
			proof, err := ingressmanifest.Preview(spec)
			if err != nil {
				t.Fatal(err)
			}
			fake := &nftLabFake{chainHandle: 7}
			runtime := NewRuntime()
			backend := &NFTLabBackend{Runner: fake, ExpectedChainHandle: 7}
			tx := &NFTLabTransaction{Runtime: runtime, Firewall: backend, Journal: new(LabJournal)}
			if err := tx.Apply(context.Background(), spec, proof); err != nil {
				t.Fatal(err)
			}
			defer runtime.Retire(spec.Config.Listen, proof)
			fake.failCommand = true
			if err := tx.Retire(context.Background(), spec, proof); err == nil {
				t.Fatal("failed delete claimed success")
			}
			fake.failCommand = false
			commands := len(fake.commands)
			if change == "runtime" {
				tx.Runtime = NewRuntime()
			} else {
				tx.Firewall = &NFTLabBackend{Runner: fake, ExpectedChainHandle: 7}
			}
			if err := tx.Retire(context.Background(), spec, proof); !errors.Is(err, ErrUnverified) {
				t.Fatalf("changed executor inherited stop proof: %v", err)
			}
			if len(fake.commands) != commands || fake.ruleHandle == 0 {
				t.Fatal("changed executor deleted rule")
			}
			tx.Runtime, tx.Firewall = runtime, backend
			if err := tx.Retire(context.Background(), spec, proof); err != nil {
				t.Fatal(err)
			}
		})
	}
}
