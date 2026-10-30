//go:build linux

package ingressownership

import (
	"context"
	"errors"
	"net"
	"strconv"
	"syscall"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressinventory"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

func TestOperationalRetireExactOwnedKernelInodeNeverWaivesForeignListener(t *testing.T) {
	ir, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ir.Addr().(*net.TCPAddr).Port
	_ = ir.Close()
	spec := ingressmanifest.Spec{TunnelID: "owned-retire-kernel", InstanceID: "ir-a", Generation: 1,
		Config: ingressmanifest.Config{Listen: net.JoinHostPort("127.0.0.1", strconv.Itoa(port)),
			Target: "127.0.0.1:1443", MaxClients: 128, DialTimeoutMS: 3000}}
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	journal := new(LabJournal)
	fake := &nftLabFake{chainHandle: 7}
	nft := &NFTLabBackend{Runner: fake, ExpectedChainHandle: 7}
	tx := &NFTLabTransaction{Runtime: rt, Firewall: nft, Journal: journal}
	if err := tx.Apply(context.Background(), spec, proof); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Retire(context.Background(), spec, proof) }()
	x := &OperationalExecutor{transaction: tx}
	b := executorCommandV1(t).Binding
	b.Spec, b.Proof, b.Port, b.ConfigPort, b.ListenEndpoint = spec, proof, uint16(port), uint16(port), spec.Config.Listen
	rt.mu.Lock()
	owned := rt.sessions[spec.Config.Listen].listener.(*net.TCPListener)
	rt.mu.Unlock()
	fd, err := owned.File()
	if err != nil {
		t.Fatal(err)
	}
	st, err := fd.Stat()
	_ = fd.Close()
	if err != nil {
		t.Fatal(err)
	}
	inode := st.Sys().(*syscall.Stat_t).Ino
	observed := ingressinventory.CoverageEvidence{Port: uint16(port), CoverageComplete: true, ActiveTCP: []ingressinventory.ActiveTCPClaim{
		{Port: uint16(port), Family: "tcp4", State: "0A", Inode: inode},
	}}
	if err := x.ownedRetireListener(context.Background(), b, observed); err != nil {
		t.Fatalf("actual live owner misclassified foreign: %v", err)
	}
	unknown := observed
	unknown.UnknownSources = []string{"uninspectable systemd/runtime source"}
	if err := x.ownedRetireListener(context.Background(), b, unknown); !errors.Is(err, ingressinventory.ErrCoverageIncomplete) {
		t.Fatalf("owned inode waived unknown source: %v", err)
	}
	incomplete := observed
	incomplete.CoverageComplete = false
	if err := x.ownedRetireListener(context.Background(), b, incomplete); !errors.Is(err, ingressinventory.ErrCoverageIncomplete) {
		t.Fatalf("owned inode waived incomplete scope: %v", err)
	}
	bad := observed
	bad.ActiveTCP = []ingressinventory.ActiveTCPClaim{{Port: uint16(port), Family: "tcp4", State: "0A", Inode: inode + 1}}
	if err := x.ownedRetireListener(context.Background(), b, bad); !errors.Is(err, ErrConflict) {
		t.Fatalf("unrelated inode was accepted: %v", err)
	}
	bad = observed
	bad.ActiveTCP = append(append([]ingressinventory.ActiveTCPClaim{}, observed.ActiveTCP...), ingressinventory.ActiveTCPClaim{
		Port: uint16(port), Family: "tcp6", State: "0A", Inode: inode + 100})
	if err := x.ownedRetireListener(context.Background(), b, bad); !errors.Is(err, ErrConflict) {
		t.Fatalf("second same-port IPv6 inode was waived: %v", err)
	}
	bad = observed
	bad.Stopped = []ingressinventory.PersistentTCPPortClaim{{Port: uint16(port), Unit: "foreign-stopped.socket"}}
	if err := x.ownedRetireListener(context.Background(), b, bad); !errors.Is(err, ErrConflict) {
		t.Fatalf("stopped foreign owner was waived: %v", err)
	}
	wrong := b
	wrong.Proof.Generation++
	if err := x.ownedRetireListener(context.Background(), wrong, observed); err == nil {
		t.Fatal("forged generation was allowed to adopt own listener")
	}
	foreign, err := net.Listen("tcp6", net.JoinHostPort("::1", strconv.Itoa(port)))
	if err == nil {
		defer foreign.Close()
		if err := x.ownedRetireListener(context.Background(), b, observed); !errors.Is(err, ErrConflict) {
			t.Fatalf("actual foreign cross-family socket escaped live validation: %v", err)
		}
	}
}
