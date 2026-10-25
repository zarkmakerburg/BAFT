package ingressownership

import (
	"context"
	"errors"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressinventory"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// Actual socket lifecycle with injected nft/journal failure; this is a
// classification test, not a positive normal signed operational Retire.
func TestOperationalStoppedRetireClassificationRequiresPrivateExactEvidence(t *testing.T) {
	for _, fault := range []string{"delete", "journal"} {
		t.Run(fault, func(t *testing.T) {
			ctx := context.Background()
			spec := testSpec(t, "operational-retry")
			proof, err := ingressmanifest.Preview(spec)
			if err != nil {
				t.Fatal(err)
			}
			fake := &nftLabFake{chainHandle: 7}
			journal := &failRetiredJournal{MutationJournal: new(LabJournal)}
			rt := NewRuntime()
			backend := &NFTLabBackend{Runner: fake, ExpectedChainHandle: 7}
			tx := &NFTLabTransaction{Runtime: rt, Firewall: backend, Journal: journal}
			if err := tx.Apply(ctx, spec, proof); err != nil {
				t.Fatal(err)
			}
			defer rt.Retire(spec.Config.Listen, proof)
			if fault == "delete" {
				fake.failCommand = true
			} else {
				journal.fail = true
			}
			if err := tx.Retire(ctx, spec, proof); err == nil {
				t.Fatal("missing retirement fault")
			}
			fake.failCommand, journal.fail = false, false
			record, found, err := journal.Current(proof)
			if err != nil || !found || record.Phase != PhaseRetireIntent {
				t.Fatalf("interrupted state: %+v %v", record, err)
			}
			b := executorCommandV1(t).Binding
			b.Spec, b.Proof, b.Port, b.ConfigPort, b.ListenEndpoint = spec, proof, record.Port, record.Port, spec.Config.Listen
			observed := ingressinventory.CoverageEvidence{Port: record.Port, CoverageComplete: true}
			x := &OperationalExecutor{transaction: tx}
			commands := len(fake.commands)
			if err := x.reconcileStoppedRetire(ctx, b, observed); err != nil {
				t.Fatalf("same-process stopped-owner retry rejected: %v", err)
			}
			if len(fake.commands) != commands {
				t.Fatal("classification mutated nft")
			}
			for _, change := range []string{"restart", "runtime", "backend", "chain", "handle", "unknown", "incomplete", "live", "stopped", "binding", "cancel"} {
				t.Run(change, func(t *testing.T) {
					candidate, observation, binding := x, observed, b
					checkCtx, cancel := context.WithCancel(ctx)
					defer cancel()
					switch change {
					case "restart":
						candidate = &OperationalExecutor{transaction: &NFTLabTransaction{Runtime: rt, Firewall: backend, Journal: journal}}
					case "runtime":
						tx.Runtime = NewRuntime()
						defer func() { tx.Runtime = rt }()
					case "backend":
						tx.Firewall = &NFTLabBackend{Runner: fake, ExpectedChainHandle: 7}
						defer func() { tx.Firewall = backend }()
					case "chain":
						backend.ExpectedChainHandle++
						defer func() { backend.ExpectedChainHandle-- }()
					case "handle":
						fake.ruleHandle++
						defer func() { fake.ruleHandle-- }()
					case "unknown":
						observation.UnknownSources = []string{"unknown"}
					case "incomplete":
						observation.CoverageComplete = false
					case "live":
						observation.ActiveTCP = []ingressinventory.ActiveTCPClaim{{Port: b.Port, State: "0A", Family: "tcp6", Inode: 123}}
					case "stopped":
						observation.Stopped = []ingressinventory.PersistentTCPPortClaim{{Port: b.Port, Unit: "foreign.socket"}}
					case "binding":
						binding.Proof.Generation++
					case "cancel":
						cancel()
					}
					if err := candidate.reconcileStoppedRetire(checkCtx, binding, observation); err == nil {
						t.Fatal("ambiguous stopped recovery accepted")
					}
					if len(fake.commands) != commands {
						t.Fatal("classification mutated nft")
					}
				})
			}
			if err := tx.Retire(ctx, spec, proof); err != nil {
				t.Fatal(err)
			}
			if err := x.reconcileStoppedRetire(ctx, b, observed); !errors.Is(err, ErrUnverified) {
				t.Fatalf("released evidence reused: %v", err)
			}
		})
	}
}
