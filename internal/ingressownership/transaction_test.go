package ingressownership

import (
	"context"
	"errors"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"net"
	"strconv"
	"testing"
)

func TestLabTransactionApplyRetireAndFirewallDrift(t *testing.T) {
	tx := &LabTransaction{Runtime: NewRuntime(), Firewall: NewMemoryFirewall()}
	s := testSpec(t, "tx")
	p, err := ingressmanifest.Preview(s)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Apply(context.Background(), s, p); err != nil {
		t.Fatal(err)
	}
	if err = tx.Apply(context.Background(), s, p); err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(s.Config.Listen)
	n, _ := strconv.Atoi(port)
	if err = tx.Firewall.Remove(uint16(n), p); err != nil {
		t.Fatal(err)
	}
	if err = tx.Apply(context.Background(), s, p); !errors.Is(err, ErrFirewallUnverified) {
		t.Fatalf("drift accepted: %v", err)
	}
	if err = tx.Retire(s.Config.Listen, p); !errors.Is(err, ErrFirewallUnverified) {
		t.Fatalf("retire without rule: %v", err)
	}
	if err = tx.Firewall.Install(uint16(n), p); err != nil {
		t.Fatal(err)
	}
	if err = tx.Retire(s.Config.Listen, p); err != nil {
		t.Fatal(err)
	}
}
func TestLabTransactionForeignFirewallRollback(t *testing.T) {
	tx := &LabTransaction{Runtime: NewRuntime(), Firewall: NewMemoryFirewall()}
	s := testSpec(t, "tx-conflict")
	p, err := ingressmanifest.Preview(s)
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(s.Config.Listen)
	n, _ := strconv.Atoi(port)
	foreign := proof("foreign", 1)
	if err = tx.Firewall.Install(uint16(n), foreign); err != nil {
		t.Fatal(err)
	}
	if err = tx.Apply(context.Background(), s, p); !errors.Is(err, ErrConflict) {
		t.Fatalf("foreign firewall: %v", err)
	}
	ok, err := tx.Firewall.Verify(uint16(n), foreign)
	if err != nil || !ok {
		t.Fatal("foreign firewall altered")
	}
	ln, err := net.Listen("tcp", s.Config.Listen)
	if err != nil {
		t.Fatalf("listener leaked after rollback: %v", err)
	}
	ln.Close()
}

func TestLabTransactionCanonicalAlias(t *testing.T) {
	tx := &LabTransaction{Runtime: NewRuntime(), Firewall: NewMemoryFirewall()}
	s := testSpec(t, "tx-alias")
	p, err := ingressmanifest.Preview(s)
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(s.Config.Listen)
	if err != nil {
		t.Fatal(err)
	}
	alias := s
	alias.Config.Listen = "127.0.0.1:0" + port
	if err = tx.Apply(context.Background(), s, p); err != nil {
		t.Fatal(err)
	}
	if err = tx.Apply(context.Background(), alias, p); err != nil {
		t.Fatal(err)
	}
	if err = tx.Retire(alias.Config.Listen, p); err != nil {
		t.Fatal(err)
	}
}

func TestLabTransactionCancelledParentDoesNotLeaveFirewall(t *testing.T) {
	tx := &LabTransaction{Runtime: NewRuntime(), Firewall: NewMemoryFirewall()}
	s := testSpec(t, "tx-cancel")
	p, err := ingressmanifest.Preview(s)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = tx.Apply(ctx, s, p); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel accepted: %v", err)
	}
	if len(tx.Runtime.sessions) != 0 {
		t.Fatal("cancelled session leaked")
	}
	_, port, _ := net.SplitHostPort(s.Config.Listen)
	n, _ := strconv.Atoi(port)
	ok, err := tx.Firewall.Verify(uint16(n), p)
	if err != nil || ok {
		t.Fatalf("cancelled firewall leaked: %v %v", ok, err)
	}
}

func TestLabTransactionRejectsNilContextWithoutMutation(t *testing.T) {
	tx := &LabTransaction{Runtime: NewRuntime(), Firewall: NewMemoryFirewall()}
	spec := testSpec(t, "tx-nil-context")
	owner, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Apply(nil, spec, owner); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nil context: %v", err)
	}
	if len(tx.Runtime.sessions) != 0 {
		t.Fatal("nil context left session")
	}
}

func TestLabTransactionCancelledReplayPreservesLiveOwner(t *testing.T) {
	tx := &LabTransaction{Runtime: NewRuntime(), Firewall: NewMemoryFirewall()}
	spec := testSpec(t, "tx-cancel-replay")
	owner, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Apply(context.Background(), spec, owner); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := tx.Apply(ctx, spec, owner); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled replay: %v", err)
	}
	_, port, _ := net.SplitHostPort(spec.Config.Listen)
	n, _ := strconv.Atoi(port)
	if ok, err := tx.Firewall.Verify(uint16(n), owner); err != nil || !ok {
		t.Fatalf("cancelled replay changed firewall: %v %v", ok, err)
	}
	if err := tx.Retire(spec.Config.Listen, owner); err != nil {
		t.Fatal(err)
	}
}
