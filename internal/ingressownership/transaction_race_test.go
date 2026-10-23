package ingressownership

import (
	"context"
	"errors"
	"net"
	"strconv"
	"sync"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

func TestLabTransactionConcurrentApplyAndRetire(t *testing.T) {
	tx := &LabTransaction{Runtime: NewRuntime(), Firewall: NewMemoryFirewall()}
	spec := testSpec(t, "tx-parallel")
	owner, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- tx.Apply(context.Background(), spec, owner) }()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("concurrent apply: %v", err)
		}
	}
	if len(tx.Runtime.sessions) != 1 {
		t.Fatalf("unexpected session count: %d", len(tx.Runtime.sessions))
	}
	_, port, err := net.SplitHostPort(spec.Config.Listen)
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := tx.Firewall.Verify(uint16(n), owner)
	if err != nil || !ok {
		t.Fatal("owner firewall missing")
	}
	if err := tx.Retire(spec.Config.Listen, owner); err != nil {
		t.Fatal(err)
	}
	if err := tx.Apply(context.Background(), spec, owner); !errors.Is(err, ErrStale) {
		t.Fatalf("replayed retired generation: %v", err)
	}
}

func TestLabTransactionConcurrentRetire(t *testing.T) {
	tx := &LabTransaction{Runtime: NewRuntime(), Firewall: NewMemoryFirewall()}
	spec := testSpec(t, "tx-retire-parallel")
	owner, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Apply(context.Background(), spec, owner); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- tx.Retire(spec.Config.Listen, owner) }()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrFirewallUnverified) {
			t.Fatalf("unexpected retire error: %v", err)
		}
	}
	if success != 1 {
		t.Fatalf("successful retires: %d", success)
	}
}
