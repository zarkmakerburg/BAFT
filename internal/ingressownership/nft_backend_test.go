package ingressownership

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

type nftLabFake struct {
	chainHandle uint64
	port        uint16
	tag         string
	ruleHandle  uint64
	reads       int
	commands    []string
	failRead    bool
	failCommand bool
	partialAdd  bool
	partialDel  bool
	swapOnRead  int
}

func (f *nftLabFake) ReadRuleset(context.Context) ([]byte, error) {
	f.reads++
	if f.failRead {
		return nil, errors.New("read timeout")
	}
	if f.swapOnRead == f.reads {
		f.ruleHandle++
	}
	raw := fmt.Sprintf(`{"nftables":[{"chain":{"family":"inet","table":"baft","name":"ingress","handle":%d,"type":"filter","hook":"input","prio":-100,"policy":"accept"}}`, f.chainHandle)
	if f.ruleHandle != 0 {
		raw += fmt.Sprintf(`,{"rule":{"family":"inet","table":"baft","chain":"ingress","handle":%d,"comment":"%s","expr":[{"match":{"op":"==","left":{"payload":{"protocol":"tcp","field":"dport"}},"right":%d}},{"accept":null}]}}`, f.ruleHandle, f.tag, f.port)
	}
	return []byte(raw + `]}`), nil
}

func (f *nftLabFake) Run(_ context.Context, args ...string) error {
	f.commands = append(f.commands, strings.Join(args, " "))
	if f.failCommand {
		return errors.New("command failed after uncertain partial write")
	}
	if len(args) > 1 && args[0] == "add" {
		port, err := strconv.Atoi(args[7])
		if err != nil {
			return err
		}
		f.port = uint16(port)
		f.tag = args[10]
		f.ruleHandle = 42
		if f.partialAdd {
			f.partialAdd = false
			return errors.New("connection lost after kernel accepted add")
		}
	} else if len(args) > 1 && args[0] == "delete" {
		f.ruleHandle = 0
		if f.partialDel {
			return errors.New("connection lost after kernel accepted delete")
		}
	} else {
		return errors.New("unexpected nft operation")
	}
	return nil
}

func TestNFTLabBackendPartialCommandRetryDoesNotDuplicateOrClaimRemoval(t *testing.T) {
	plan, _ := PlanIngressRule(54619, proof("nft-partial", 1))
	fake := &nftLabFake{chainHandle: 7, partialAdd: true}
	backend := &NFTLabBackend{Runner: fake, ExpectedChainHandle: 7}
	if _, err := backend.Install(context.Background(), plan); err == nil || fake.ruleHandle == 0 {
		t.Fatalf("lost partial add uncertainty: %v", err)
	}
	if observed, err := backend.Install(context.Background(), plan); err != nil || observed.Handle != 42 || len(fake.commands) != 1 {
		t.Fatalf("partial add retry duplicated rule: %+v %v %v", observed, err, fake.commands)
	}
	fake.partialDel = true
	if err := backend.Remove(context.Background(), plan, 42); err == nil {
		t.Fatal("partial delete reported success")
	}
	if err := backend.Remove(context.Background(), plan, 42); !errors.Is(err, ErrFirewallUnverified) {
		t.Fatalf("partial delete retry adopted absent rule: %v", err)
	}
}

func TestNFTLabBackendCancelledContextNeverMutates(t *testing.T) {
	plan, _ := PlanIngressRule(54619, proof("nft-cancel", 1))
	fake := &nftLabFake{chainHandle: 7}
	backend := &NFTLabBackend{Runner: fake, ExpectedChainHandle: 7}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := backend.Install(ctx, plan); !errors.Is(err, context.Canceled) || len(fake.commands) != 0 {
		t.Fatalf("cancelled install mutated: %v %v", err, fake.commands)
	}
	fake.port, fake.tag, fake.ruleHandle = plan.Port, plan.Tag, 42
	if err := backend.Remove(ctx, plan, 42); !errors.Is(err, context.Canceled) || len(fake.commands) != 0 {
		t.Fatalf("cancelled remove mutated: %v %v", err, fake.commands)
	}
}

func TestNFTLabBackendExactOwnerInstallAndRemove(t *testing.T) {
	plan, err := PlanIngressRule(54619, proof("nft-backend", 1))
	if err != nil {
		t.Fatal(err)
	}
	fake := &nftLabFake{chainHandle: 7}
	backend := &NFTLabBackend{Runner: fake, ExpectedChainHandle: 7}
	observed, err := backend.Install(context.Background(), plan)
	if err != nil || observed.Handle != 42 || len(fake.commands) != 1 {
		t.Fatalf("install: %+v %v commands=%v", observed, err, fake.commands)
	}
	if observed, err := backend.Install(context.Background(), plan); err != nil || observed.Handle != 42 || len(fake.commands) != 1 {
		t.Fatalf("idempotent install: %+v %v commands=%v", observed, err, fake.commands)
	}
	if err := backend.Remove(context.Background(), plan, 42); err != nil || len(fake.commands) != 2 {
		t.Fatalf("remove: %v commands=%v", err, fake.commands)
	}
	if err := backend.Remove(context.Background(), plan, 42); !errors.Is(err, ErrFirewallUnverified) {
		t.Fatalf("accepted removal without independent rule evidence: %v", err)
	}
}

func TestNFTLabBackendFailsClosedOnConflictSwapAndFailure(t *testing.T) {
	plan, _ := PlanIngressRule(54619, proof("nft-negative", 1))
	foreign := &nftLabFake{chainHandle: 7, port: plan.Port, tag: "baft-ingress-foreign", ruleHandle: 42}
	backend := &NFTLabBackend{Runner: foreign, ExpectedChainHandle: 7}
	if _, err := backend.Install(context.Background(), plan); !errors.Is(err, ErrConflict) || len(foreign.commands) != 0 {
		t.Fatalf("accepted port conflict: %v", err)
	}
	swap := &nftLabFake{chainHandle: 7, port: plan.Port, tag: plan.Tag, ruleHandle: 42, swapOnRead: 2}
	backend.Runner = swap
	if err := backend.Remove(context.Background(), plan, 42); !errors.Is(err, ErrFirewallUnverified) || len(swap.commands) != 0 {
		t.Fatalf("deleted swapped handle: %v", err)
	}
	for _, bad := range []*nftLabFake{
		{chainHandle: 8},
		{chainHandle: 7, failRead: true},
		{chainHandle: 7, failCommand: true},
	} {
		backend.Runner = bad
		if _, err := backend.Install(context.Background(), plan); err == nil {
			t.Fatalf("accepted invalid chain/read/command: %+v", bad)
		}
	}
}
