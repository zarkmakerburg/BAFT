package ingressownership

import (
	"context"
	"errors"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"net"
	"testing"
	"time"
)

func testSpec(t *testing.T, owner string) ingressmanifest.Spec {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := ln.Addr().String()
	ln.Close()
	return ingressmanifest.Spec{TunnelID: owner, Generation: 1, Config: ingressmanifest.Config{Listen: address, Target: "127.0.0.1:1443"}}
}
func TestRuntimeApplyRetireOwnership(t *testing.T) {
	r := NewRuntime()
	s := testSpec(t, "a")
	p, err := ingressmanifest.Preview(s)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Apply(context.Background(), s, p); err != nil {
		t.Fatal(err)
	}
	if err = r.Apply(context.Background(), s, p); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if err = r.Retire(s.Config.Listen, proof("b", 1)); !errors.Is(err, ErrUnknown) {
		t.Fatalf("foreign retire: %v", err)
	}
	c, err := net.DialTimeout("tcp", s.Config.Listen, time.Second)
	if err != nil {
		t.Fatalf("foreign retire closed socket: %v", err)
	}
	c.Close()
	if err = r.Retire(s.Config.Listen, p); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", s.Config.Listen)
	if err != nil {
		t.Fatalf("retire leaked socket: %v", err)
	}
	ln.Close()
	if err = r.Apply(context.Background(), s, p); !errors.Is(err, ErrStale) {
		t.Fatalf("replay: %v", err)
	}
}
func TestRuntimeBindConflictPreservesForeignSocket(t *testing.T) {
	r := NewRuntime()
	s := testSpec(t, "a")
	p, err := ingressmanifest.Preview(s)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", s.Config.Listen)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if err = r.Apply(context.Background(), s, p); err == nil {
		t.Fatal("accepted foreign bind")
	}
	if err = r.Retire(s.Config.Listen, p); !errors.Is(err, ErrUnknown) {
		t.Fatalf("foreign retire: %v", err)
	}
	c, err := net.DialTimeout("tcp", s.Config.Listen, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
}

func TestRuntimeBindFailureDoesNotConsumeGeneration(t *testing.T) {
	r := NewRuntime()
	s := testSpec(t, "bind-retry")
	p, err := ingressmanifest.Preview(s)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := net.Listen("tcp", s.Config.Listen)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Apply(context.Background(), s, p); err == nil {
		t.Fatal("foreign bind unexpectedly succeeded")
	}
	if err = foreign.Close(); err != nil {
		t.Fatal(err)
	}
	if err = r.Apply(context.Background(), s, p); err != nil {
		t.Fatalf("failed bind consumed generation: %v", err)
	}
	if err = r.Retire(s.Config.Listen, p); err != nil {
		t.Fatal(err)
	}
}
