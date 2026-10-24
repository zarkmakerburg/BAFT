//go:build linux

package publicingress_test

// Release gate operational-ingress-v1: the real listener + nft entry. Runs only
// inside the disposable private network namespace (tests/release/opgate/run_lab.sh);
// it never touches the host network, files or firewall.

import (
	"bytes"
	"context"
	"io"
	"net"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"github.com/zarkmakerburg/baft/internal/ingressownership"
	"github.com/zarkmakerburg/baft/internal/publicingress"
	"github.com/zarkmakerburg/baft/tests/e2e/operational/oplab"
)

func nft(t *testing.T, args ...string) []byte {
	t.Helper()
	out, err := exec.Command(oplab.NFT, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("nft %v: %v: %s", args, err, out)
	}
	return out
}

func TestOperationalIngressV1Gate_RealListenerAndNFTInPrivateNetns(t *testing.T) {
	oplab.RequireNetns(t)
	if out, err := exec.Command(oplab.IPBinary(), "link", "set", "lo", "up").CombinedOutput(); err != nil {
		t.Fatalf("lo up: %v: %s", err, out)
	}
	_ = exec.Command(oplab.NFT, "delete", "table", "inet", "baft").Run()
	nft(t, "add", "table", "inet", "baft")
	t.Cleanup(func() { _ = exec.Command(oplab.NFT, "delete", "table", "inet", "baft").Run() })
	nft(t, "add", "chain", "inet", "baft", "ingress", "{ type filter hook input priority -100; policy accept; }")
	nft(t, "add", "table", "inet", "unrelated")
	t.Cleanup(func() { _ = exec.Command(oplab.NFT, "delete", "table", "inet", "unrelated").Run() })
	nft(t, "add", "chain", "inet", "unrelated", "keep")
	nft(t, "add", "rule", "inet", "unrelated", "keep", "tcp", "dport", "2053", "accept")
	unrelatedBefore := nft(t, "-j", "list", "table", "inet", "unrelated")
	handle, err := ingressownership.ReadNFTLabChainHandle(nft(t, "-j", "list", "ruleset"))
	if err != nil {
		t.Fatal(err)
	}

	// The tunnel target: counts and echoes.
	target, err := net.Listen("tcp4", "127.0.0.1:54821")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	var received bytes.Buffer
	var mu sync.Mutex
	go func() {
		for {
			c, err := target.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(10 * time.Second))
				buf := make([]byte, 4096)
				for {
					n, err := c.Read(buf)
					if n > 0 {
						mu.Lock()
						received.Write(buf[:n])
						mu.Unlock()
						_, _ = c.Write(buf[:n])
					}
					if err != nil {
						return
					}
				}
			}()
		}
	}()
	unrelatedSvc, err := net.Listen("tcp4", "127.0.0.1:2053")
	if err != nil {
		t.Fatal(err)
	}
	defer unrelatedSvc.Close()

	spec := ingressmanifest.Spec{TunnelID: "tun-nft-gate", Generation: 1, Config: ingressmanifest.Config{Listen: "0.0.0.0:54820", Target: "127.0.0.1:54821", MaxClients: 16, DialTimeoutMS: 1000}}
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := ingressownership.PlanIngressRule(54820, proof)
	if err != nil {
		t.Fatal(err)
	}
	journalDir := t.TempDir()
	journal, err := ingressownership.OpenDurableJournal(journalDir)
	if err != nil {
		t.Fatal(err)
	}
	backend := &ingressownership.NFTLabBackend{Runner: ingressownership.NFTCommandRunner{Binary: oplab.NFT}, ExpectedChainHandle: handle}
	tx := &ingressownership.NFTLabTransaction{Runtime: ingressownership.NewRuntime(), Firewall: backend, Journal: journal}

	// A foreign claim on the numeric port refuses the whole transaction
	// before any rule exists.
	foreign, err := net.Listen("tcp6", "[::]:54820")
	if err != nil {
		t.Fatalf("ipv6 loopback is required in the LAB: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := tx.Apply(ctx, spec, proof); err == nil {
		t.Fatal("Apply succeeded next to a foreign IPv6 listener on the same numeric port")
	}
	if bytes.Contains(nft(t, "-j", "list", "chain", "inet", "baft", "ingress"), []byte(plan.Tag)) {
		t.Fatal("a rule was installed over a foreign claim")
	}
	_ = foreign.Close()

	// Real Apply: kernel listener first, owner-tagged nft rule after.
	if err := tx.Apply(ctx, spec, proof); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := nft(t, "-j", "list", "chain", "inet", "baft", "ingress"); !bytes.Contains(got, []byte(plan.Tag)) || !bytes.Contains(got, []byte("54820")) {
		t.Fatalf("owned rule missing: %s", got)
	}
	if rule, err := backend.Inspect(ctx, plan); err != nil || rule.Handle == 0 {
		t.Fatalf("exact handle readback: %+v %v", rule, err)
	}
	// Client bytes cross the real listener to the target and back.
	payload := bytes.Repeat([]byte("nft-gate-"), 1000)
	c, err := net.DialTimeout("tcp4", "127.0.0.1:54820", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write(payload); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(c, got); err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("round trip: %v", err)
	}
	_ = c.Close()
	// The forwarder itself answers the BCC probe frame (never reaching the target).
	frame := publicingress.ProbePrefix + strings.Repeat("a1", publicingress.ProbeNonceLen/2) + "\n"
	pc, err := net.DialTimeout("tcp4", "127.0.0.1:54820", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = pc.SetDeadline(time.Now().Add(3 * time.Second))
	_, _ = pc.Write([]byte(frame))
	echoed := make([]byte, len(frame))
	if _, err := io.ReadFull(pc, echoed); err != nil || string(echoed) != frame {
		t.Fatalf("probe not answered by the forwarder: %v %q", err, echoed)
	}
	_ = pc.Close()
	mu.Lock()
	seen := received.Len()
	mu.Unlock()
	if seen != len(payload) {
		t.Fatalf("target saw %d bytes, want exactly the client payload (%d); the probe must not reach it", seen, len(payload))
	}
	// Idempotent replay does not duplicate the rule.
	if err := tx.Apply(ctx, spec, proof); err != nil {
		t.Fatalf("idempotent Apply: %v", err)
	}
	if n := strings.Count(string(nft(t, "-j", "list", "chain", "inet", "baft", "ingress")), plan.Tag); n == 0 || n > 3 {
		t.Fatalf("rule duplicated: %d", n)
	}

	// Retire removes exactly the owned rule and closes exactly the listener.
	if err := tx.Retire(ctx, spec, proof); err != nil {
		t.Fatalf("Retire: %v", err)
	}
	if bytes.Contains(nft(t, "-j", "list", "chain", "inet", "baft", "ingress"), []byte(plan.Tag)) {
		t.Fatal("owned rule survived Retire")
	}
	if l, err := net.Listen("tcp4", "0.0.0.0:54820"); err != nil {
		t.Fatalf("listener survived Retire: %v", err)
	} else {
		_ = l.Close()
	}
	if after := nft(t, "-j", "list", "table", "inet", "unrelated"); !bytes.Equal(after, unrelatedBefore) {
		t.Fatal("unrelated nft table changed")
	}
	if c, err := net.DialTimeout("tcp4", "127.0.0.1:2053", time.Second); err != nil {
		t.Fatalf("unrelated service on 2053 lost: %v", err)
	} else {
		_ = c.Close()
	}
}
