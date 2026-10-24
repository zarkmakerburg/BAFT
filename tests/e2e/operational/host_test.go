//go:build linux

package operational

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressownership"
	"github.com/zarkmakerburg/baft/tests/e2e/operational/oplab"
)

const (
	irNode  = "ir-opv1"
	irAddr  = "10.77.0.1"
	adminTk = "admin-opv1-token"
)

func nodeToken(id string) string { return "tok-" + id }

// host is the disposable private-netns IR host: loopback + one alias address,
// a pre-provisioned inet/baft/ingress nft chain and the protected managed-node
// policy (inside the private mount namespace created by oplab).
type host struct {
	t      *testing.T
	handle uint64
}

func sh(t testing.TB, name string, args ...string) []byte {
	t.Helper()
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, out)
	}
	return out
}

func newHost(t *testing.T) *host {
	t.Helper()
	oplab.Require(t)
	ip := oplab.IPBinary()
	sh(t, ip, "link", "set", "lo", "up")
	if have := sh(t, ip, "-4", "addr", "show", "dev", "lo"); !strings.Contains(string(have), irAddr+"/32") {
		sh(t, ip, "addr", "add", irAddr+"/32", "dev", "lo")
	}
	h := &host{t: t}
	_ = exec.Command(oplab.NFT, "delete", "table", "inet", "baft").Run()
	sh(t, oplab.NFT, "add", "table", "inet", "baft")
	t.Cleanup(func() { _ = exec.Command(oplab.NFT, "delete", "table", "inet", "baft").Run() })
	sh(t, oplab.NFT, "add", "chain", "inet", "baft", "ingress", "{ type filter hook input priority -100; policy accept; }")
	raw := sh(t, oplab.NFT, "-j", "list", "ruleset")
	handle, err := ingressownership.ReadNFTLabChainHandle(raw)
	if err != nil {
		t.Fatalf("pinned baft chain: %v", err)
	}
	h.handle = handle
	if _, err := ingressauthority.ProvisionInstalledManagedNodePolicy(context.Background(), irNode); err != nil {
		t.Fatalf("provision installed policy: %v", err)
	}
	return h
}

// ruleset returns the JSON of the baft ingress chain.
func (h *host) chainJSON() string {
	return string(sh(h.t, oplab.NFT, "-j", "list", "chain", "inet", "baft", "ingress"))
}

// baftRules counts the owner-tagged rules in the baft chain.
func (h *host) baftRules() int { return strings.Count(h.chainJSON(), "baft-ingress-") }

// addKeep installs an unrelated table+rule that must survive everything.
func (h *host) addKeep() (snapshot func() string) {
	sh(h.t, oplab.NFT, "add", "table", "inet", "op_keep")
	h.t.Cleanup(func() { _ = exec.Command(oplab.NFT, "delete", "table", "inet", "op_keep").Run() })
	sh(h.t, oplab.NFT, "add", "chain", "inet", "op_keep", "keep")
	sh(h.t, oplab.NFT, "add", "rule", "inet", "op_keep", "keep", "tcp", "dport", "2053", "accept")
	return func() string { return string(sh(h.t, oplab.NFT, "-j", "list", "table", "inet", "op_keep")) }
}

// echo is a loopback TCP server (the stand-in for the IR tunnel entry that the
// ingress forwarder dials) counting every byte it echoes.
type echo struct {
	ln    net.Listener
	mu    sync.Mutex
	bytes int
	wg    sync.WaitGroup
}

func startEcho(t testing.TB, addr string) *echo {
	t.Helper()
	ln, err := net.Listen("tcp4", addr)
	if err != nil {
		t.Fatalf("echo listen %s: %v", addr, err)
	}
	e := &echo{ln: ln}
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			e.wg.Add(1)
			go func() {
				defer e.wg.Done()
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(60 * time.Second))
				buf := make([]byte, 4096)
				for {
					n, err := c.Read(buf)
					if n > 0 {
						e.mu.Lock()
						e.bytes += n
						e.mu.Unlock()
						if _, werr := c.Write(buf[:n]); werr != nil {
							return
						}
					}
					if err != nil {
						return
					}
				}
			}()
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return e
}

func (e *echo) seen() int { e.mu.Lock(); defer e.mu.Unlock(); return e.bytes }

// roundTrip sends payload through addr and requires the exact echo back.
func roundTrip(t testing.TB, addr string, payload []byte) error {
	t.Helper()
	c, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write(payload); err != nil {
		return err
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(c, got); err != nil {
		return err
	}
	if !bytes.Equal(got, payload) {
		return fmt.Errorf("echo mismatch")
	}
	return nil
}

// portFree reports whether nothing listens on the numeric TCP port (v4+v6).
func portFree(port int) bool {
	for _, a := range []string{"0.0.0.0", "[::]"} {
		ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", a, port))
		if err != nil {
			return false
		}
		_ = ln.Close()
	}
	return true
}

func listening(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// stoppedUnit writes a systemd .socket unit (a stopped foreign service) into
// the private /etc/systemd/system copy.
func stoppedUnit(t testing.TB, name, listen string) {
	t.Helper()
	path := filepath.Join("/etc/systemd/system", name+".socket")
	if err := os.WriteFile(path, []byte("[Socket]\nListenStream="+listen+"\n\n[Install]\nWantedBy=sockets.target\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
}

// foreignListen opens a real foreign listener.
func foreignListen(t testing.TB, network, addr string) net.Listener {
	t.Helper()
	ln, err := net.Listen(network, addr)
	if err != nil {
		t.Fatalf("foreign listen %s %s: %v", network, addr, err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln
}

var _ = ingressauthority.ProtocolV1

func mkdirPrivate(p string) error {
	if err := os.MkdirAll(p, 0700); err != nil {
		return err
	}
	return os.Chmod(p, 0700)
}
