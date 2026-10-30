//go:build linux

package ingressownership

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

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

type injectAfterFirewallInstallJournal struct {
	MutationJournal
	hook func()
}

func (j *injectAfterFirewallInstallJournal) Advance(next MutationRecord) error {
	if err := j.MutationJournal.Advance(next); err != nil {
		return err
	}
	if next.Phase == PhaseFirewallInstalled && j.hook != nil {
		hook := j.hook
		j.hook = nil
		hook()
	}
	return nil
}

// The foreign socket may appear DURING firewall installation, before Apply
// returns an ACK. Even a stopped session with a recorded terminal failure
// cannot strand the exact-owned allow rule during failed Apply rollback.
func TestLabApplyRollsBackExactOwnedFirewallOnForeignPortDuringInstallLinux(t *testing.T) {
	probe, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(probe.Addr().(*net.TCPAddr).Port)
	probe.Close()
	spec := ingressmanifest.Spec{TunnelID: "tx-install-late-foreign", Generation: 1, Config: ingressmanifest.Config{Listen: net.JoinHostPort("127.0.0.1", port), Target: "127.0.0.1:1443"}}
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	fw := NewMemoryFirewall()
	j := &injectAfterFirewallInstallJournal{MutationJournal: new(LabJournal)}
	tx := &LabTransaction{Runtime: NewRuntime(), Firewall: fw, Journal: j}
	var foreign net.Listener
	j.hook = func() {
		var listenErr error
		foreign, listenErr = net.Listen("tcp6", net.JoinHostPort("::1", port))
		if listenErr != nil {
			t.Skipf("kernel does not permit cross-family same-port listeners: %v", listenErr)
		}
		if err := tx.Runtime.Apply(context.Background(), spec, proof); !errors.Is(err, ErrStopped) {
			t.Errorf("lost foreign owner was not fenced: %v", err)
		}
		// Deliver a deterministic already-completed stopped session before
		// the transaction's post-install health check.
		tx.Runtime.mu.Lock()
		s := tx.Runtime.sessions[spec.Config.Listen]
		tx.Runtime.mu.Unlock()
		if s == nil {
			t.Error("missing session")
			return
		}
		select {
		case <-s.done:
		case <-time.After(3 * time.Second):
			t.Error("ownership-loss stop did not complete")
		}
	}
	defer func() {
		if foreign != nil {
			foreign.Close()
		}
	}()
	err = tx.Apply(context.Background(), spec, proof)
	if foreign == nil {
		t.Skip("kernel does not permit cross-family same-port listeners (no IPv6 loopback)")
	}
	if err == nil {
		t.Fatal("unexpected successful Apply after foreign listener arrived during installation")
	}
	portNum, _ := strconv.Atoi(port)
	if exists, verifyErr := fw.Verify(uint16(portNum), proof); verifyErr != nil || exists {
		t.Fatalf("failed Apply leaked owned allow rule: exists=%v verify=%v apply=%v", exists, verifyErr, err)
	}
	if len(tx.Runtime.sessions) != 0 {
		t.Fatal("failed Apply retained old listener owner")
	}
	if c, err := net.Dial("tcp6", foreign.Addr().String()); err != nil {
		t.Fatalf("failed Apply damaged foreign service: %v", err)
	} else {
		c.Close()
	}
}
