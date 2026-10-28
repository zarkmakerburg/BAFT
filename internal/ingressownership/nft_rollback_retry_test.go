package ingressownership

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

type rollbackIntentFaultJournal struct {
	MutationJournal
	fail bool
}

func (j *rollbackIntentFaultJournal) Advance(next MutationRecord) error {
	if j.fail && next.Phase == PhaseRetireIntent {
		return errFinalizeFault
	}
	return j.MutationJournal.Advance(next)
}

func TestNFTLabFailedApplyCleanupCanRetireExactRuleInSameProcess(t *testing.T) {
	spec := testSpec(t, "rollback-delete-retry")
	owner, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	fake := &nftLabFake{chainHandle: 7, partialAdd: true}
	journal := new(LabJournal)
	backend := &NFTLabBackend{Runner: &partialAddFaultRunner{nftLabFake: fake, fault: "delete"}, ExpectedChainHandle: 7}
	tx := &NFTLabTransaction{Runtime: NewRuntime(), Firewall: backend, Journal: journal}
	if err := tx.Apply(context.Background(), spec, owner); err == nil {
		t.Fatal("failed apply claimed success")
	}
	defer tx.Runtime.Retire(spec.Config.Listen, owner)
	foreign, err := listenWithForensics(t, "tcp4", spec.Config.Listen)
	if err != nil {
		t.Fatal(err)
	}
	defer foreign.Close()
	fake.failCommand = false
	commands := len(fake.commands)
	restarted := &NFTLabTransaction{Runtime: NewRuntime(), Firewall: backend, Journal: journal}
	if err := restarted.Retire(context.Background(), spec, owner); err == nil {
		t.Fatal("restart adopted rollback journal")
	}
	if len(fake.commands) != commands || fake.ruleHandle == 0 {
		t.Fatal("restart changed uncertain rule")
	}
	if err := tx.Retire(context.Background(), spec, owner); err != nil {
		t.Fatalf("same-process exact rollback could not finish: %v", err)
	}
	current, _, err := journal.Current(owner)
	if err != nil || current.Phase != PhaseRetired || fake.ruleHandle != 0 {
		t.Fatalf("rollback not finalized: %+v %v", current, err)
	}
	c, err := net.Dial("tcp4", foreign.Addr().String())
	if err != nil {
		t.Fatalf("rollback retry touched replacement listener: %v", err)
	}
	_ = c.Close()
	commands = len(fake.commands)
	if err := tx.Apply(context.Background(), spec, owner); err == nil || len(fake.commands) != commands {
		t.Fatal("rollback resurrected failed generation")
	}
}

func TestNFTLabRollbackIntentRequiresUnchangedLiveProof(t *testing.T) {
	for _, fault := range []string{"runtime", "backend", "chain-pin", "handle", "owner", "read", "cancel"} {
		t.Run(fault, func(t *testing.T) {
			spec := testSpec(t, "rollback-intent-fence")
			owner, err := ingressmanifest.Preview(spec)
			if err != nil {
				t.Fatal(err)
			}
			fake := &nftLabFake{chainHandle: 7, partialAdd: true}
			journal := &rollbackIntentFaultJournal{MutationJournal: new(LabJournal), fail: true}
			runtime := NewRuntime()
			backend := &NFTLabBackend{Runner: fake, ExpectedChainHandle: 7}
			tx := &NFTLabTransaction{Runtime: runtime, Firewall: backend, Journal: journal}
			if err := tx.Apply(context.Background(), spec, owner); err == nil {
				t.Fatal("failed Apply acknowledged success")
			}
			defer runtime.Retire(spec.Config.Listen, owner)
			tag := fake.tag
			journal.fail = false
			commands := len(fake.commands)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch fault {
			case "runtime":
				tx.Runtime = NewRuntime()
			case "backend":
				tx.Firewall = &NFTLabBackend{Runner: fake, ExpectedChainHandle: 7}
			case "chain-pin":
				fake.chainHandle, backend.ExpectedChainHandle = 8, 8
			case "handle":
				fake.ruleHandle = 43
			case "owner":
				fake.tag = "baft-ingress-" + strings.Repeat("a", 32)
			case "read":
				fake.failRead = true
			case "cancel":
				cancel()
			}
			if err := tx.Retire(ctx, spec, owner); err == nil {
				t.Fatal("changed rollback proof admitted cleanup")
			}
			current, _, err := journal.Current(owner)
			if err != nil || current.Phase != PhaseListenerBound || current.FirewallHandle != 0 || len(fake.commands) != commands {
				t.Fatalf("invalid proof changed state: %+v %v", current, err)
			}
			tx.Runtime, tx.Firewall = runtime, backend
			fake.chainHandle, backend.ExpectedChainHandle = 7, 7
			fake.ruleHandle, fake.tag, fake.failRead = 42, tag, false
			if err := tx.Retire(context.Background(), spec, owner); err != nil {
				t.Fatalf("valid rollback could not finish: %v", err)
			}
		})
	}
}
