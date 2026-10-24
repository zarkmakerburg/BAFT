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

// A stopped ingress may have a terminal ownership failure, even though exact
// socket and generation release SUCCEEDED. The transaction must not leave an
// owned firewall rule behind merely because Retire reports that failure.
func TestLabTransactionRemovesOwnedFirewallAfterRetiredListenerConflictLinux(t *testing.T) {
	probe, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(probe.Addr().(*net.TCPAddr).Port)
	probe.Close()
	spec := ingressmanifest.Spec{TunnelID: "tx-foreign-cleanup", Generation: 1, Config: ingressmanifest.Config{Listen: net.JoinHostPort("127.0.0.1", port), Target: "127.0.0.1:1443"}}
	p, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	fw := NewMemoryFirewall()
	j := new(LabJournal)
	tx := &LabTransaction{Runtime: NewRuntime(), Firewall: fw, Journal: j}
	if err := tx.Apply(context.Background(), spec, p); err != nil {
		t.Fatal(err)
	}
	foreign, err := net.Listen("tcp6", net.JoinHostPort("::1", port))
	if err != nil {
		t.Skipf("independent family unsupported: %v", err)
	}
	defer foreign.Close()
	replayErr := tx.Apply(context.Background(), spec, p)
	if replayErr == nil || (!errors.Is(replayErr, ErrStopped) && !strings.Contains(replayErr.Error(), "already held by another live TCP listener")) {
		t.Fatalf("replay must immediately reject foreign listener: %v", replayErr)
	}
	retireErr := tx.Retire(spec.Config.Listen, p)
	// A completed stopped session can have a nil terminal cause when
	// canceled concurrently; if a particular replay has concrete ownership
	// evidence, exact retirement must not silently discard that evidence.
	if strings.Contains(replayErr.Error(), "already held by another live TCP listener") && (retireErr == nil || !strings.Contains(retireErr.Error(), "already held by another live TCP listener")) {
		t.Fatalf("retirement discarded witnessed ownership failure: %v", retireErr)
	}
	portNo, _ := strconv.Atoi(port)
	owned, verifyErr := fw.Verify(uint16(portNo), p)
	if verifyErr != nil || owned {
		t.Fatalf("owned firewall rule survived verified socket release: owned=%v err=%v", owned, verifyErr)
	}
	current, found, err := j.Current(p)
	if err != nil || !found || current.Phase != PhaseRetired {
		t.Fatalf("cleanup journal still advertises active firewall: %+v %v", current, err)
	}
	if len(tx.Runtime.sessions) != 0 {
		t.Fatal("retirement leaked runtime sessions")
	}
	if conn, err := net.Dial("tcp6", foreign.Addr().String()); err != nil {
		t.Fatalf("cleanup damaged unrelated listener: %v", err)
	} else {
		_ = conn.Close()
	}
}

// The nft journal must likewise reach PhaseRetired when Runtime confirms that
// it released the exact socket and registry proof, even if the session stopped
// because another live owner appeared. Removal still requires the exact
// independently observed nft rule handle and owner tag.
func TestNFTLabTransactionRemovesExactOwnedRuleAfterRetiredConflictLinux(t *testing.T) {
	probe, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(probe.Addr().(*net.TCPAddr).Port)
	probe.Close()
	spec := ingressmanifest.Spec{TunnelID: "nft-foreign-cleanup", Generation: 1, Config: ingressmanifest.Config{Listen: net.JoinHostPort("127.0.0.1", port), Target: "127.0.0.1:1443"}}
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	fake := &nftLabFake{chainHandle: 7}
	j := new(LabJournal)
	tx := &NFTLabTransaction{Runtime: NewRuntime(), Firewall: &NFTLabBackend{Runner: fake, ExpectedChainHandle: 7}, Journal: j}
	if err := tx.Apply(context.Background(), spec, proof); err != nil {
		t.Fatal(err)
	}
	foreign, err := net.Listen("tcp6", net.JoinHostPort("::1", port))
	if err != nil {
		t.Skipf("independent family unavailable: %v", err)
	}
	defer foreign.Close()
	replayErr := tx.Apply(context.Background(), spec, proof)
	if replayErr == nil || (!errors.Is(replayErr, ErrStopped) && !strings.Contains(replayErr.Error(), "another live TCP listener")) {
		t.Fatalf("NFT replay wrongly reported healthy: %v", replayErr)
	}
	retireErr := tx.Retire(context.Background(), spec, proof)
	if strings.Contains(replayErr.Error(), "another live TCP listener") && (retireErr == nil || !strings.Contains(retireErr.Error(), "another live TCP listener")) {
		t.Fatalf("nft cleanup discarded witnessed terminal cause: %v", retireErr)
	}
	if fake.ruleHandle != 0 {
		t.Fatalf("verified retired socket left exact-owned firewall rule handle: %d", fake.ruleHandle)
	}
	if current, found, err := j.Current(proof); err != nil || !found || current.Phase != PhaseRetired {
		t.Fatalf("nft journal did not commit cleanup: %+v found=%v err=%v", current, found, err)
	}
	if len(fake.commands) != 2 || !strings.HasPrefix(fake.commands[1], "delete ") {
		t.Fatalf("failed to remove exact owned nft rule: %+v", fake.commands)
	}
	if len(tx.Runtime.sessions) != 0 {
		t.Fatal("nft cleanup leaked runtime session")
	}
	if c, err := net.Dial("tcp6", foreign.Addr().String()); err != nil {
		t.Fatalf("nft retirement interrupted unrelated socket: %v", err)
	} else {
		c.Close()
	}
}
