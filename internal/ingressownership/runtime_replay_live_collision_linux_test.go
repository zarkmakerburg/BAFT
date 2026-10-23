//go:build linux

package ingressownership

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// The periodic watchdog catches a late foreign socket only on its next tick.
// An idempotent replay/health ACK must not report a just-contended port as
// healthy in that gap: replay must immediately recheck live ownership.
func TestRuntimeReplayChecksCurrentLivePortOwnerBeforeHealthyACKLinux(t *testing.T) {
	ir, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	irPort := strconv.Itoa(ir.Addr().(*net.TCPAddr).Port)
	ir.Close()
	spec := ingressmanifest.Spec{
		TunnelID: "health-foreign-race", Generation: 1,
		Config: ingressmanifest.Config{Listen: net.JoinHostPort("127.0.0.1", irPort), Target: "127.0.0.1:1443"},
	}
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	r := NewRuntime()
	if err := r.Apply(context.Background(), spec, proof); err != nil {
		t.Fatal(err)
	}
	// A different family can legally bind the same numeric port on Linux.
	foreign, err := net.Listen("tcp6", net.JoinHostPort("::1", irPort))
	if err != nil {
		_ = r.Retire(spec.Config.Listen, proof)
		t.Skipf("kernel refuses independent IPv6 address: %v", err)
	}
	defer foreign.Close()
	// Do NOT wait for the 1s watcher. Health must fail closed immediately.
	err = r.Apply(context.Background(), spec, proof)
	if !errors.Is(err, ErrStopped) || !strings.Contains(err.Error(), "another live TCP listener") {
		t.Fatalf("idempotent replay falsely ACKed contended port as healthy: %v", err)
	}
	if err = r.Apply(context.Background(), spec, proof); !errors.Is(err, ErrStopped) {
		t.Fatalf("repeated replay revived lost owner: %v", err)
	}
	if err = r.Retire(spec.Config.Listen, proof); err == nil || !strings.Contains(err.Error(), "another live TCP listener") {
		t.Fatalf("retire lost original ownership-loss evidence: %v", err)
	}
	// A forced in-memory retirement must not disturb a foreign service.
	if conn, err := net.Dial("tcp6", foreign.Addr().String()); err != nil {
		t.Fatalf("foreign listener destroyed by our retirement: %v", err)
	} else {
		_ = conn.Close()
	}
	if len(r.sessions) != 0 {
		t.Fatal("retirement leaked session")
	}
	_ = foreign.Close()
	next := spec
	next.Generation = 2
	nextProof, err := ingressmanifest.Preview(next)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Apply(context.Background(), next, nextProof); err != nil {
		t.Fatalf("properly retired port was not recyclable with newer generation: %v", err)
	}
	if err := r.Retire(next.Config.Listen, nextProof); err != nil {
		t.Fatal(err)
	}
}
