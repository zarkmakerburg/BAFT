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

// Decommissioning one independent instance must not block every later
// instance on the same node. Retire removes config, unit and marker and
// releases the metrics claim, leaving only the instance directory and a
// complete retire receipt; that is a retired sibling, not an unverifiable
// one (guided-flow-e2e host leg, run 41281786914: second tunnel on the same
// EX ended rollback_failed with "sibling instance has no verifiable config").
func TestRetiredInstanceDoesNotBlockLaterInstances(t *testing.T) {
	ctx := context.Background()
	p := newPair(t)
	exBase, irBase := p.ex.Manager, p.ir.Manager
	p.ex.Manager = scoped(t, p.ex, "first", "127.0.0.1:9831")
	p.ir.Manager = scoped(t, p.ir, "first", "127.0.0.1:9832")
	p.build(t, "t-first")
	for _, n := range []*node{p.ex, p.ir} {
		if _, err := n.Finalize(ctx, "t-first"); err != nil {
			t.Fatal(err)
		}
	}
	for name, n := range map[string]*node{"ex": p.ex, "ir": p.ir} {
		before := n.Inspect(ctx)
		if _, err := n.Retire(ctx, "t-first", retireWant(before)); err != nil {
			t.Fatalf("%s retire: %v", name, err)
		}
	}
	for name, base := range map[string]*Manager{"ex": exBase, "ir": irBase} {
		if _, err := base.ForInstance("second", "127.0.0.1:9833"); err != nil {
			t.Fatalf("%s: a retired instance blocks later instances: %v", name, err)
		}
	}
	// A retired directory that regains a unit is not "retired" any more.
	first := scoped(t, &node{Manager: exBase}, "first", "127.0.0.1:9831")
	if err := os.MkdirAll(first.UnitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first.unitPath(), []byte("[Unit]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := exBase.ForInstance("third", "127.0.0.1:9834"); err == nil {
		t.Fatal("a sibling with a unit but no config was accepted")
	}
}

// When preparation is refused by the sibling fence nothing is created, so the
// compensating rollback must succeed instead of failing on the same fence
// (which turned a refused build into rollback_failed). The fence still guards
// every new allocation.
func TestRollbackIsNotBlockedBySiblingFence(t *testing.T) {
	ctx := context.Background()
	n := newNode(t)
	if err := os.MkdirAll(filepath.Join(n.Manager.ConfigDir, "instances", "unverifiable"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := n.Manager.ForInstance("fresh", "127.0.0.1:9841"); err == nil {
		t.Fatal("the sibling fence no longer guards new allocations")
	}
	m, err := n.Manager.ForInstanceNoAllocation("fresh", "127.0.0.1:9841")
	if err != nil {
		t.Fatalf("rollback scope refused: %v", err)
	}
	if out, err := m.Rollback(ctx, "never-prepared"); err != nil || out != "nothing to roll back" {
		t.Fatalf("rollback of a refused prepare: %q %v", out, err)
	}
	for _, bad := range []string{"", "127.0.0.1:x", "10.0.0.1:9841"} {
		if _, err := n.Manager.ForInstanceNoAllocation("fresh", bad); err == nil {
			t.Fatalf("metrics listener %q accepted without validation", bad)
		}
	}
}
