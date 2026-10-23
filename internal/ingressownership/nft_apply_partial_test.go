package ingressownership

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

type partialAddFaultRunner struct {
	*nftLabFake
	fault string
}

func (r *partialAddFaultRunner) Run(ctx context.Context, args ...string) error {
	err := r.nftLabFake.Run(ctx, args...)
	if len(args) != 0 && args[0] == "add" {
		switch r.fault {
		case "read":
			r.failRead = true
		case "owner":
			r.tag = "baft-ingress-" + strings.Repeat("a", 32)
		case "handle-swap":
			r.swapOnRead = r.reads + 2
		case "delete":
			r.failCommand = true
		}
	}
	return err
}

func TestNFTLabApplyPartialAddCleanupFailureRemainsUnverified(t *testing.T) {
	spec := testSpec(t, "partial-cleanup-failure")
	owner, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	fake := &nftLabFake{chainHandle: 7, partialAdd: true}
	journal := new(LabJournal)
	tx := &NFTLabTransaction{Runtime: NewRuntime(), Firewall: &NFTLabBackend{Runner: &partialAddFaultRunner{nftLabFake: fake, fault: "delete"}, ExpectedChainHandle: 7}, Journal: journal}
	if err := tx.Apply(context.Background(), spec, owner); err == nil {
		t.Fatal("failed cleanup reported success")
	}
	defer tx.Runtime.Retire(spec.Config.Listen, owner)
	current, _, err := journal.Current(owner)
	if err != nil || current.Phase != PhaseListenerBound || current.FirewallHandle != 0 || fake.ruleHandle != 42 {
		t.Fatalf("lost cleanup uncertainty: %+v %v", current, err)
	}
	ln, err := net.Listen("tcp4", spec.Config.Listen)
	if err != nil {
		t.Fatalf("cleanup failure left original listener: %v", err)
	}
	_ = ln.Close()
	// This increment deliberately does not grant restart adoption or claim
	// automatic recovery from a second failure during rollback.
	if err := tx.Retire(context.Background(), spec, owner); err == nil {
		t.Fatal("unverified rollback journal authorized retirement")
	}
}

func TestNFTLabApplyRollsBackIndependentlyObservedPartialAdd(t *testing.T) {
	spec := testSpec(t, "partial-add-rollback")
	owner, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	fake := &nftLabFake{chainHandle: 7, partialAdd: true}
	journal := new(LabJournal)
	tx := &NFTLabTransaction{Runtime: NewRuntime(), Firewall: &NFTLabBackend{Runner: fake, ExpectedChainHandle: 7}, Journal: journal}
	if err := tx.Apply(context.Background(), spec, owner); err == nil {
		t.Fatal("partial add acknowledged success")
	}
	defer tx.Runtime.Retire(spec.Config.Listen, owner)
	ln, err := net.Listen("tcp4", spec.Config.Listen)
	if err != nil {
		t.Fatalf("failed apply left listener: %v", err)
	}
	defer ln.Close()
	if fake.ruleHandle != 0 {
		t.Fatal("failed apply leaked independently observable exact-owned rule")
	}
	current, _, err := journal.Current(owner)
	if err != nil || current.Phase == PhaseVerified {
		t.Fatalf("failed apply recorded success: %+v %v", current, err)
	}
}

func TestNFTLabApplyPartialAddRollbackRefusesUnverifiedResources(t *testing.T) {
	for _, fault := range []string{"preflight", "foreign", "read", "owner", "handle-swap"} {
		t.Run(fault, func(t *testing.T) {
			spec := testSpec(t, "partial-add-fence")
			owner, err := ingressmanifest.Preview(spec)
			if err != nil {
				t.Fatal(err)
			}
			fake := &nftLabFake{chainHandle: 7, partialAdd: true}
			if fault == "preflight" {
				fake.failRead, fake.ruleHandle = true, 88
			}
			if fault == "foreign" {
				_, plan, err := nftTransactionRecord(spec, owner)
				if err != nil {
					t.Fatal(err)
				}
				fake.port, fake.tag, fake.ruleHandle = plan.Port, "baft-ingress-"+strings.Repeat("a", 32), 88
			}
			runner := &partialAddFaultRunner{nftLabFake: fake, fault: fault}
			tx := &NFTLabTransaction{Runtime: NewRuntime(), Firewall: &NFTLabBackend{Runner: runner, ExpectedChainHandle: 7}, Journal: new(LabJournal)}
			if err := tx.Apply(context.Background(), spec, owner); err == nil {
				t.Fatal("unverified apply claimed success")
			}
			defer tx.Runtime.Retire(spec.Config.Listen, owner)
			if fake.ruleHandle == 0 {
				t.Fatal("rollback deleted unverified rule")
			}
			for _, command := range fake.commands {
				if strings.HasPrefix(command, "delete ") {
					t.Fatalf("unverified deletion: %s", command)
				}
			}
			ln, err := net.Listen("tcp4", spec.Config.Listen)
			if err != nil {
				t.Fatalf("failed apply left listener: %v", err)
			}
			_ = ln.Close()
		})
	}
}
