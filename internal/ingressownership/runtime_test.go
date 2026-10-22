package ingressownership

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
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

func TestRuntimeCancelledParentRequiresExplicitRetire(t *testing.T) {
	r := NewRuntime()
	s := testSpec(t, "cancel-parent")
	p, err := ingressmanifest.Preview(s)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err = r.Apply(ctx, s, p); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err = r.Apply(context.Background(), s, p); err != nil && !errors.Is(err, ErrStopped) {
		t.Fatalf("retry after cancellation must not create new socket: %v", err)
	}
	if err = r.Retire(s.Config.Listen, p); err != nil {
		t.Fatal(err)
	}
	if err = r.Apply(context.Background(), s, p); !errors.Is(err, ErrStale) {
		t.Fatalf("cancelled generation replay: %v", err)
	}
}
func TestRuntimeConcurrentRetireCannotReleaseNewOwner(t *testing.T) {
	r := NewRuntime()
	s := testSpec(t, "retire-race")
	p, err := ingressmanifest.Preview(s)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Apply(context.Background(), s, p); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- r.Retire(s.Config.Listen, p) }()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrUnknown) && !errors.Is(err, ErrConflict) {
			t.Fatalf("unexpected retire result: %v", err)
		}
	}
	if success != 1 {
		t.Fatalf("successful retires=%d", success)
	}
}

func TestRuntimeStoppedSessionCannotReportHealthyIdempotentApply(t *testing.T) {
	r := NewRuntime()
	s := testSpec(t, "stopped-health")
	p, err := ingressmanifest.Preview(s)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err = r.Apply(ctx, s, p); err != nil {
		t.Fatal(err)
	}
	cancel()
	deadline := time.Now().Add(3 * time.Second)
	for {
		err = r.Apply(context.Background(), s, p)
		if errors.Is(err, ErrStopped) {
			break
		}
		if err != nil {
			t.Fatalf("unexpected stopped retry error: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatal("stopped session still reported healthy")
		}
		time.Sleep(time.Millisecond)
	}
	if err = r.Retire(s.Config.Listen, p); err != nil {
		t.Fatal(err)
	}
}
