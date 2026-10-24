package ingressownership

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

func TestRuntimeIPv4WildcardForwardsAndRetires(t *testing.T) {
	target, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		c, err := target.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(time.Second))
		_, _ = c.Write([]byte("wildcard-ok"))
	}()
	defer func() { _ = target.Close(); <-done }()
	s := testSpec(t, "ipv4-wildcard")
	_, port, err := net.SplitHostPort(s.Config.Listen)
	if err != nil {
		t.Fatal(err)
	}
	s.Config.Listen = net.JoinHostPort("0.0.0.0", port)
	s.Config.Target = target.Addr().String()
	p, err := ingressmanifest.Preview(s)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRuntime()
	if err := r.Apply(context.Background(), s, p); err != nil {
		t.Fatal(err)
	}
	// Keep an identity for the original bound TCP socket BEFORE Retire:
	// a subsequent bind to this numeric port cannot distinguish an actual
	// leaked listener from an unrelated service that acquired the freed port.
	r.mu.Lock()
	owned := r.sessions[s.Config.Listen].listener.(*net.TCPListener)
	r.mu.Unlock()
	assertKernelClosed := ownedListenerKernelClosureCheck(t, owned)
	defer r.Retire(s.Config.Listen, p)
	c, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", port), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = c.SetDeadline(time.Now().Add(time.Second))
	got := make([]byte, len("wildcard-ok"))
	_, err = io.ReadFull(c, got)
	_ = c.Close()
	if err != nil || string(got) != "wildcard-ok" {
		t.Fatalf("IPv4 wildcard forwarding: %q %v", got, err)
	}
	if err := r.Apply(context.Background(), s, p); err != nil {
		t.Fatalf("live wildcard retry: %v", err)
	}
	if err := r.Retire(s.Config.Listen, p); err != nil {
		t.Fatal(err)
	}
	// A completed Retire must close the *original socket identity*, rather
	// than assume this port remains free after retirement. Other processes
	// can legitimately bind it immediately and cause EADDRINUSE.
	if file, err := owned.File(); err == nil {
		_ = file.Close()
		t.Fatal("retired wildcard listener still exposes a live socket FD")
	} else if !errors.Is(err, net.ErrClosed) {
		t.Fatalf("retired listener File did not prove a closed socket: %v", err)
	}
	assertKernelClosed()
}
