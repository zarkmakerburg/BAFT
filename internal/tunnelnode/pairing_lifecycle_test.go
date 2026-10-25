package tunnelnode

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func scoped(t *testing.T, n *node, id, metrics string) *Manager {
	t.Helper()
	m, err := n.Manager.ForInstance(id, metrics)
	if err != nil {
		t.Fatalf("ForInstance(%s): %v", id, err)
	}
	return m
}

func claimPath(m *Manager, port string) string {
	return filepath.Join(filepath.Dir(filepath.Dir(m.ConfigDir)), ".metrics-claims", port)
}

// A pairing that is cancelled before commit must not leave the node unable to
// add any other instance: the abandoned instance directory holds a key but no
// config, which the sibling preflight treated as an unverifiable sibling.
func TestCancelledPairingDoesNotBlockLaterInstances(t *testing.T) {
	ctx := context.Background()
	n := newNode(t)
	a := scoped(t, n, "abandoned", "127.0.0.1:9811")
	if _, err := a.PrepareEX(ctx, "p1", ExParams{PublicAddress: "127.0.0.1", Port: freePort(t), Target: "127.0.0.1:2443", RouteID: "r"}); err != nil {
		t.Fatal(err)
	}
	if out, err := a.Rollback(ctx, "p1"); err != nil {
		t.Fatalf("cancel: %v %s", err, out)
	}
	b, err := n.Manager.ForInstance("next", "127.0.0.1:9812")
	if err != nil {
		t.Fatalf("a cancelled pairing blocks every later instance: %v", err)
	}
	if _, err := b.PrepareEX(ctx, "p2", ExParams{PublicAddress: "127.0.0.1", Port: freePort(t), Target: "127.0.0.1:2443", RouteID: "r"}); err != nil {
		t.Fatal(err)
	}
}

// A rejected pairing code (malformed, wrong prefix, empty) must fail closed
// without leaving an active marker or a metrics claim, and must not block
// other instances nor a retry of the same instance with a fresh id.
func TestRejectedPairingCodeLeavesNodeReusable(t *testing.T) {
	ctx := context.Background()
	n := newNode(t)
	a := scoped(t, n, "badcode", "127.0.0.1:9821")
	for i, code := range []string{"BAFTPAIR1:not-base64!!", "BAFTPAIR0:x", "garbage"} {
		id := "p" + strconv.Itoa(i)
		_, err := a.PrepareIR(ctx, id, code, IRParams{RouteListen: "127.0.0.1:" + strconv.Itoa(freePort(t)), RouteID: "r"})
		if err == nil {
			t.Fatalf("code %q accepted", code)
		}
		if strings.Contains(err.Error(), code) {
			t.Fatalf("error leaks code: %v", err)
		}
		if a.activeID() != "" {
			t.Fatalf("failed prepare left active marker for %q", code)
		}
		if _, err := os.Stat(claimPath(a, "9821")); !os.IsNotExist(err) {
			t.Fatalf("failed prepare kept metrics claim: %v", err)
		}
		if _, err := os.Stat(a.liveConfig()); !os.IsNotExist(err) {
			t.Fatalf("failed prepare wrote a live config: %v", err)
		}
	}
	if _, err := n.Manager.ForInstance("another", "127.0.0.1:9822"); err != nil {
		t.Fatalf("failed pairing blocks later instances: %v", err)
	}
}
