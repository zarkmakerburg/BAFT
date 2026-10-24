//go:build linux

package ingressownership

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

func TestRuntimeApplyRefusesLiveCrossFamilyOwnerBeforeAcknowledging(t *testing.T) {
	foreign, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer foreign.Close()
	port := strconv.Itoa(foreign.Addr().(*net.TCPAddr).Port)
	// A separately bound IPv6 address may use this same port even though
	// an existing IPv4 service already owns it.
	s := ingressmanifest.Spec{
		TunnelID: "alias-live-owner", Generation: 1,
		Config: ingressmanifest.Config{Listen: net.JoinHostPort("::1", port), Target: "127.0.0.1:1443"},
	}
	proof, err := ingressmanifest.Preview(s)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRuntime()
	err = r.Apply(context.Background(), s, proof)
	if err == nil {
		t.Fatal("Apply acknowledged live foreign port collision")
	}
	if !strings.Contains(err.Error(), "another live TCP listener") {
		t.Fatalf("expected real foreign port ownership rejection, got: %v", err)
	}
	if len(r.sessions) != 0 {
		t.Fatal("failed admission created a managed session")
	}
	// Foreign service remains available and unchanged.
	client, err := net.DialTimeout("tcp4", foreign.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("foreign listener was disrupted: %v", err)
	}
	_ = foreign.(*net.TCPListener).SetDeadline(time.Now().Add(time.Second))
	accepted, err := foreign.Accept()
	if err != nil {
		t.Fatalf("foreign service stopped accepting: %v", err)
	}
	accepted.Close()
	client.Close()

	// A failed observed collision cannot burn the legitimate generation.
	foreign.Close()
	if err = r.Apply(context.Background(), s, proof); err != nil {
		t.Fatalf("failed admission consumed generation: %v", err)
	}
	if err = r.Retire(s.Config.Listen, proof); err != nil {
		t.Fatalf("unable to retire recovered generation: %v", err)
	}
}
