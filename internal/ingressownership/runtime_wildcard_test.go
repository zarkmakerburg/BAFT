package ingressownership

import (
	"context"
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
	rebound, err := net.Listen("tcp4", s.Config.Listen)
	if err != nil {
		t.Fatalf("retire leaked wildcard socket: %v", err)
	}
	_ = rebound.Close()
}
