package tunnelnode

import (
	"bytes"
	"context"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
)

// freshManager is what a restarted process (or a later CLI invocation) sees:
// a brand new Manager over the same on-disk state, with no in-memory history.
func freshManager(t *testing.T, n *node, instance, metrics string) *Manager {
	t.Helper()
	env := n.Manager.Env
	env.Instance = ""
	env.ConfigDir, env.StateDir = n.dir+"/etc", n.dir+"/state"
	env.Service = "baft"
	root, err := New(env)
	must(t, err)
	m, err := root.ForInstance(instance, metrics)
	must(t, err)
	return m
}

// An interrupted pairing (process gone between prepare and commit) is durable:
// a restarted manager refuses any other change, can finish the original one
// with the same reply, or can cancel it and then accept a new id.
func TestInterruptedPairingIsDurableAcrossRestart(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []string{"complete", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			p := newPair(t)
			ex := scoped(t, p.ex, "rs", "127.0.0.1:9851")
			ir := scoped(t, p.ir, "rs", "127.0.0.1:9852")
			code, err := ex.PrepareEX(ctx, "p1", p.exp)
			must(t, err)
			reply, err := ir.PrepareIR(ctx, "p1", code, p.irp)
			must(t, err)

			// "Crash": drop both managers, rebuild from disk.
			ex2 := freshManager(t, p.ex, "rs", "127.0.0.1:9851")
			ir2 := freshManager(t, p.ir, "rs", "127.0.0.1:9852")
			if _, err := ex2.PrepareEX(ctx, "p2", p.exp); err == nil {
				t.Fatal("restarted manager allowed a second change over an unfinished one")
			}
			if ex2.activeID() != "p1" {
				t.Fatalf("restart lost the active owner: %q", ex2.activeID())
			}
			if mode == "complete" {
				listen(t, p.exPort)
				listen(t, mustPort(p.irListen))
				if out, err := ex2.CommitEX(ctx, "p1", reply); err != nil || !strings.Contains(out, "active") {
					t.Fatalf("commit after restart: %q %v", out, err)
				}
				if _, err := ir2.CommitIR(ctx, "p1"); err != nil {
					t.Fatal(err)
				}
				if _, err := ex2.Finalize(ctx, "p1"); err != nil {
					t.Fatal(err)
				}
				return
			}
			for _, m := range []*Manager{ex2, ir2} {
				if out, err := m.Rollback(ctx, "p1"); err != nil {
					t.Fatalf("cancel after restart: %q %v", out, err)
				}
				if out, err := m.Rollback(ctx, "p1"); err != nil || out != "already rolled back" {
					t.Fatalf("cancel is not idempotent: %q %v", out, err)
				}
				if _, err := os.Stat(m.liveConfig()); !os.IsNotExist(err) {
					t.Fatalf("cancelled pairing left a live config: %v", err)
				}
			}
			// The EX pairing secret must be gone after cancellation.
			if _, err := os.Stat(ex2.stage("p1") + "/psk"); !os.IsNotExist(err) {
				t.Fatalf("cancelled pairing kept its one-time PSK: %v", err)
			}
			// The old reply can no longer finish anything.
			if _, err := ex2.CommitEX(ctx, "p1", reply); err == nil {
				t.Fatal("cancelled pairing could still be committed")
			}
			// A new id repairs the same instance.
			code2, err := ex2.PrepareEX(ctx, "p2", p.exp)
			must(t, err)
			if code2 == code {
				t.Fatal("re-pair reused the cancelled pairing code")
			}
		})
	}
}

// Re-pair / update of a finalized instance: a reply made for an older pairing
// cannot be accepted by the newer one, and rolling the update back restores
// the previous config, unit, marker, key and generation byte for byte while a
// sibling instance is never touched.
func TestRepairFinalizedInstanceRollsBackToPreviousExactly(t *testing.T) {
	ctx := context.Background()
	p := newPair(t)
	listen(t, p.exPort)
	listen(t, mustPort(p.irListen))
	exA, irA := scoped(t, p.ex, "a", "127.0.0.1:9861"), scoped(t, p.ir, "a", "127.0.0.1:9862")
	pairUp := func(ex, ir *Manager, id string, ep ExParams, ip IRParams) (string, string) {
		code, err := ex.PrepareEX(ctx, id, ep)
		must(t, err)
		reply, err := ir.PrepareIR(ctx, id, code, ip)
		must(t, err)
		if _, err := ex.CommitEX(ctx, id, reply); err != nil {
			t.Fatal(err)
		}
		if _, err := ir.CommitIR(ctx, id); err != nil {
			t.Fatal(err)
		}
		return code, reply
	}
	_, reply1 := pairUp(exA, irA, "t1", p.exp, p.irp)
	must(t, func() error { _, err := exA.Finalize(ctx, "t1"); return err }())
	must(t, func() error { _, err := irA.Finalize(ctx, "t1"); return err }())

	// A sibling instance, finalized, whose files must never change.
	exp2 := ExParams{PublicAddress: "127.0.0.1", Port: freePort(t), Target: "127.0.0.1:2443", RouteID: "s"}
	irp2 := IRParams{RouteListen: "127.0.0.1:" + strconv.Itoa(freePort(t)), RouteID: "s"}
	listen(t, exp2.Port)
	listen(t, mustPort(irp2.RouteListen))
	exS, irS := scoped(t, p.ex, "sib", "127.0.0.1:9863"), scoped(t, p.ir, "sib", "127.0.0.1:9864")
	pairUp(exS, irS, "s1", exp2, irp2)
	must(t, func() error { _, err := exS.Finalize(ctx, "s1"); return err }())
	must(t, func() error { _, err := irS.Finalize(ctx, "s1"); return err }())

	snap := func(ms ...*Manager) map[string][]byte {
		out := map[string][]byte{}
		for _, m := range ms {
			for _, f := range []string{m.liveConfig(), m.unitPath(), m.markerPath(), m.noiseKey(), m.generationPath()} {
				b, err := os.ReadFile(f)
				must(t, err)
				out[f] = b
			}
		}
		return out
	}
	before := snap(exA, irA)
	callsBefore := map[*node]int{p.ex: len(p.ex.host.calls), p.ir: len(p.ir.host.calls)}
	siblingBefore := snap(exS, irS)

	// Update: same instance, new target and a new transaction id.
	upd := p.exp
	upd.Target = "127.0.0.1:2555"
	code2, err := exA.PrepareEX(ctx, "t2", upd)
	must(t, err)
	if _, err := exA.CommitEX(ctx, "t2", reply1); err == nil {
		t.Fatal("a reply from the previous pairing was accepted by the update")
	}
	if _, err := os.Stat(exA.stage("t2") + "/psk"); err != nil {
		t.Fatalf("a rejected reply consumed the pending pairing: %v", err)
	}
	reply2, err := irA.PrepareIR(ctx, "t2", code2, p.irp)
	must(t, err)
	if _, err := exA.CommitEX(ctx, "t2", reply2); err != nil {
		t.Fatal(err)
	}
	if _, err := irA.CommitIR(ctx, "t2"); err != nil {
		t.Fatal(err)
	}
	if cfg, _ := os.ReadFile(exA.liveConfig()); !strings.Contains(string(cfg), "2555") {
		t.Fatalf("update not applied:\n%s", cfg)
	}
	for _, m := range []*Manager{exA, irA} {
		if out, err := m.Rollback(ctx, "t2"); err != nil {
			t.Fatalf("rollback update: %q %v", out, err)
		}
	}
	for path, want := range before {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("rollback of the update did not restore %s (%v)", path, err)
		}
	}
	// The live instance still depends on its metrics claim after the update
	// was cancelled; a later instance must not be able to take that port.
	for _, m := range []*Manager{exA, irA} {
		_, port, _ := net.SplitHostPort(m.MetricsListen)
		if b, err := os.ReadFile(claimPath(m, port)); err != nil || string(b) != "a\n" {
			t.Fatalf("rolling back an update released the live instance's metrics claim: %v %q", err, b)
		}
	}
	for path, want := range siblingBefore {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("sibling file changed: %s (%v)", path, err)
		}
	}
	for _, n := range []*node{p.ex, p.ir} {
		for _, c := range n.host.calls[callsBefore[n]:] {
			if strings.HasSuffix(c, "baft-sib") {
				t.Fatalf("update of one instance touched the sibling service: %q", c)
			}
		}
	}
}
