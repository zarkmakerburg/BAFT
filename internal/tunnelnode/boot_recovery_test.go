package tunnelnode

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bootFixture leaves a finalized first tunnel plus foreign files on an EX node
// and then a SECOND change that "dies" (panics) at the service restart, i.e.
// with config, unit and marker already switched but nothing verified.
type bootFixture struct {
	p          *pair
	env        Env
	exBefore   []byte
	unitBefore []byte
	markBefore []byte
	foreign    map[string]string
}

func newBootFixture(t *testing.T, crashOn string) *bootFixture {
	t.Helper()
	p := newPair(t)
	p.build(t, "first")
	ctx := context.Background()
	if _, err := p.ex.Finalize(ctx, "first"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ir.Finalize(ctx, "first"); err != nil {
		t.Fatal(err)
	}
	f := &bootFixture{p: p, foreign: map[string]string{}}
	f.exBefore, _ = os.ReadFile(p.ex.liveConfig())
	f.unitBefore, _ = os.ReadFile(p.ex.unitPath())
	f.markBefore, _ = os.ReadFile(p.ex.markerPath())
	f.foreign[filepath.Join(p.ex.UnitDir, "x-ui.service")] = "[Service]\nExecStart=/usr/bin/x-ui\n"
	f.foreign[filepath.Join(p.ex.ConfigDir, "x-ui.json")] = `{"foreign":true}`
	f.foreign[filepath.Join(p.ex.ConfigDir, "instances-note.txt")] = "operator note\n"
	for path, body := range f.foreign {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p.exp.Port = freePort(t)
	code, err := p.ex.PrepareEX(ctx, "second", p.exp)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := p.ir.PrepareIR(ctx, "second", code, p.irp)
	if err != nil {
		t.Fatal(err)
	}
	f.env = p.ex.Manager.Env
	if crashOn == "" {
		return f // prepared only
	}
	crashed, err := New(withSystem(f.env, &crashingHost{fakeHost: p.ex.host, crashOn: crashOn}))
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("the simulated crash never happened")
			}
		}()
		_, _ = crashed.CommitEX(ctx, "second", reply)
	}()
	return f
}

// reboot returns a brand-new Manager over the same directories whose service
// manager is a fresh recorder (a rebooted node).
func (f *bootFixture) reboot(t *testing.T) (*Manager, *fakeHost) {
	t.Helper()
	h := &fakeHost{}
	m, err := New(withSystem(f.env, h))
	if err != nil {
		t.Fatal(err)
	}
	return m, h
}

func (f *bootFixture) assertForeignIntact(t *testing.T) {
	t.Helper()
	for path, body := range f.foreign {
		if b, err := os.ReadFile(path); err != nil || string(b) != body {
			t.Fatalf("foreign file %s touched: %q %v", path, b, err)
		}
	}
}

func noServiceStart(t *testing.T, h *fakeHost, rep BootReport) {
	t.Helper()
	for _, c := range h.calls {
		for _, verb := range []string{"restart", "stop", "start"} {
			if strings.HasPrefix(c, verb) {
				t.Fatalf("boot recovery issued a service %s: %v", verb, h.calls)
			}
		}
	}
	_ = rep
}

// All three files switched, then death: roll FORWARD, keep the txn committed
// and the node locked for BCC, never restart anything, never touch foreign files.
func TestBootRecoveryRollsForwardACompletelyInstalledCommit(t *testing.T) {
	f := newBootFixture(t, "restart")
	m, h := f.reboot(t)
	cfgNow, _ := os.ReadFile(m.liveConfig())
	unitNow, _ := os.ReadFile(m.unitPath())
	markNow, _ := os.ReadFile(m.markerPath())
	if string(cfgNow) == string(f.exBefore) {
		t.Fatal("fixture: change did not switch the config")
	}
	rep, err := m.RecoverAtBoot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Action != BootForward || rep.TunnelID != "second" {
		t.Fatalf("report: %+v", rep)
	}
	for _, name := range []string{"config", "unit", "marker"} {
		if rep.Files[name] != evInstalled {
			t.Fatalf("evidence %s=%q: %+v", name, rep.Files[name], rep)
		}
	}
	if m.activeID() != "second" {
		t.Fatal("roll forward released the node; only BCC may finalize")
	}
	if txn, err := m.readTxn("second"); err != nil || txn.Phase != PhaseCommitted {
		t.Fatalf("txn after forward: %+v %v", txn, err)
	}
	for path, want := range map[string][]byte{m.liveConfig(): cfgNow, m.unitPath(): unitNow, m.markerPath(): markNow} {
		if got, _ := os.ReadFile(path); string(got) != string(want) {
			t.Fatalf("forward rewrote %s", path)
		}
	}
	if !h.enabled {
		t.Fatal("the committed unit was left disabled, so it would not start at boot")
	}
	noServiceStart(t, h, rep)
	f.assertForeignIntact(t)
	// Idempotent: a second boot decides the same, still touching nothing.
	again, err := m.RecoverAtBoot(context.Background())
	if err != nil || again.Action != BootForward {
		t.Fatalf("second run: %+v %v", again, err)
	}
}

// A commit that died with only some files switched is rolled back byte for byte.
func TestBootRecoveryRollsBackAPartiallyInstalledCommit(t *testing.T) {
	for _, tc := range []struct{ name, undo string }{
		{"unit-and-marker-not-yet-written", "unit+marker"},
		{"marker-not-yet-written", "marker"},
		{"config-only-switched-back", "config"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newBootFixture(t, "restart")
			m, h := f.reboot(t)
			txn, err := m.readTxn("second")
			if err != nil {
				t.Fatal(err)
			}
			// Emulate the earlier death point by putting selected files back to
			// what existed before the change (exactly what a crash earlier in
			// install() leaves behind).
			put := func(path string, body []byte) {
				if err := os.WriteFile(path, body, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if strings.Contains(tc.undo, "unit") {
				put(m.unitPath(), f.unitBefore)
			}
			if strings.Contains(tc.undo, "marker") {
				put(m.markerPath(), f.markBefore)
			}
			if tc.undo == "config" {
				put(m.liveConfig(), f.exBefore)
			}
			_ = txn
			rep, err := m.RecoverAtBoot(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if rep.Action != BootRolledBack {
				t.Fatalf("report: %+v", rep)
			}
			for path, want := range map[string][]byte{m.liveConfig(): f.exBefore, m.unitPath(): f.unitBefore, m.markerPath(): f.markBefore} {
				if got, _ := os.ReadFile(path); string(got) != string(want) {
					t.Fatalf("%s not restored byte for byte", path)
				}
			}
			if m.activeID() != "" {
				t.Fatal("node still locked after rollback")
			}
			if txn, _ := m.readTxn("second"); txn.Phase != PhaseRolledBack {
				t.Fatalf("txn phase %s", txn.Phase)
			}
			noServiceStart(t, h, rep)
			f.assertForeignIntact(t)
		})
	}
}

// A file that is neither what BAFT wrote nor what it replaced is never touched.
func TestBootRecoveryFencesForeignModifications(t *testing.T) {
	for _, which := range []string{"config", "unit", "marker"} {
		t.Run(which, func(t *testing.T) {
			f := newBootFixture(t, "restart")
			m, h := f.reboot(t)
			path := map[string]string{"config": m.liveConfig(), "unit": m.unitPath(), "marker": m.markerPath()}[which]
			if err := os.WriteFile(path, []byte("operator edited this after the crash\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			snapshot := map[string][]byte{}
			for _, p := range []string{m.liveConfig(), m.unitPath(), m.markerPath(), m.activePath(), m.txnPath("second")} {
				snapshot[p], _ = os.ReadFile(p)
			}
			rep, err := m.RecoverAtBoot(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if rep.Action != BootFenced || rep.Files[which] != evForeign {
				t.Fatalf("report: %+v", rep)
			}
			for p, want := range snapshot {
				if got, _ := os.ReadFile(p); string(got) != string(want) {
					t.Fatalf("fenced recovery changed %s", p)
				}
			}
			noServiceStart(t, h, rep)
			f.assertForeignIntact(t)
		})
	}
}

// A half-prepared change (death during pairing) is rolled back; nothing live
// was switched, the replaced PKI comes back and the node is unlocked.
func TestBootRecoveryRollsBackAHalfPreparedChange(t *testing.T) {
	f := newBootFixture(t, "")
	m, h := f.reboot(t)
	rep, err := m.RecoverAtBoot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Action != BootRolledBack || rep.TunnelID != "second" || rep.Phase != PhasePrepared {
		t.Fatalf("report: %+v", rep)
	}
	if m.activeID() != "" {
		t.Fatal("node still locked")
	}
	if got, _ := os.ReadFile(m.liveConfig()); string(got) != string(f.exBefore) {
		t.Fatal("prepared rollback changed the live config")
	}
	if got, _ := os.ReadFile(m.unitPath()); string(got) != string(f.unitBefore) {
		t.Fatal("prepared rollback changed the unit")
	}
	if _, err := os.Stat(filepath.Join(m.stage("second"), "psk")); err == nil {
		t.Fatal("one-time pairing secret survived the rollback")
	}
	noServiceStart(t, h, rep)
	f.assertForeignIntact(t)
	// The node accepts a new change again.
	if _, err := m.begin("third", RoleEX); err != nil {
		t.Fatalf("node not reusable after recovery: %v", err)
	}
}

// Corrupt or ambiguous transaction state is never "repaired".
func TestBootRecoveryFencesCorruptAndAmbiguousState(t *testing.T) {
	t.Run("corrupt-txn", func(t *testing.T) {
		f := newBootFixture(t, "restart")
		m, _ := f.reboot(t)
		if err := os.WriteFile(m.txnPath("second"), []byte("{not json"), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, _ := os.ReadFile(m.liveConfig())
		rep, err := m.RecoverAtBoot(context.Background())
		if err != nil || rep.Action != BootFenced {
			t.Fatalf("report: %+v %v", rep, err)
		}
		if got, _ := os.ReadFile(m.liveConfig()); string(got) != string(cfg) {
			t.Fatal("live config changed")
		}
		if m.activeID() != "second" {
			t.Fatal("marker removed from a node with unreadable evidence")
		}
	})
	t.Run("invalid-active-marker", func(t *testing.T) {
		f := newBootFixture(t, "restart")
		m, _ := f.reboot(t)
		if err := os.WriteFile(m.activePath(), []byte("../../etc\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		rep, _ := m.RecoverAtBoot(context.Background())
		if rep.Action != BootFenced {
			t.Fatalf("report: %+v", rep)
		}
	})
	t.Run("marker-names-another-change", func(t *testing.T) {
		f := newBootFixture(t, "restart")
		m, _ := f.reboot(t)
		if err := os.WriteFile(m.activePath(), []byte("other\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		rep, _ := m.RecoverAtBoot(context.Background())
		if rep.Action != BootFenced {
			t.Fatalf("report: %+v", rep)
		}
		if _, err := os.Stat(m.txnPath("second")); err != nil {
			t.Fatal(err)
		}
	})
}

// The window inside begin(): marker claimed, txn.json not yet written.
func TestBootRecoveryClearsAnOrphanClaimOnlyWhenNothingExists(t *testing.T) {
	n := newNode(t)
	if err := os.MkdirAll(filepath.Join(n.StateDir, "tunnels", "ghost"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(n.activePath(), []byte("ghost\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err := n.RecoverAtBoot(context.Background())
	if err != nil || rep.Action != BootRolledBack {
		t.Fatalf("report: %+v %v", rep, err)
	}
	if n.activeID() != "" {
		t.Fatal("orphan claim kept")
	}
	// With any file in the stage the evidence is insufficient.
	if err := os.MkdirAll(filepath.Join(n.StateDir, "tunnels", "ghost2"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(n.StateDir, "tunnels", "ghost2", "psk"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(n.activePath(), []byte("ghost2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rep, _ = n.RecoverAtBoot(context.Background())
	if rep.Action != BootFenced || n.activeID() != "ghost2" {
		t.Fatalf("report: %+v", rep)
	}
}

func TestBootRecoveryBusyLifecycleLockChangesNothing(t *testing.T) {
	f := newBootFixture(t, "restart")
	m, _ := f.reboot(t)
	other, _ := f.reboot(t)
	unlock, err := other.lockLifecycle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	cfg, _ := os.ReadFile(m.liveConfig())
	if _, err := m.RecoverAtBoot(context.Background()); err == nil {
		t.Fatal("recovery ran while another process held the lifecycle lock")
	}
	if got, _ := os.ReadFile(m.liveConfig()); string(got) != string(cfg) || m.activeID() != "second" {
		t.Fatal("busy recovery changed state")
	}
}

// Recovery of one instance never reads or writes a sibling instance.
func TestBootRecoveryAllInstancesAreIndependent(t *testing.T) {
	n := newNode(t)
	ctx := context.Background()
	a, err := n.Manager.ForInstance("slot-a", "127.0.0.1:9291")
	if err != nil {
		t.Fatal(err)
	}
	a.User = ""
	b, err := n.Manager.ForInstance("slot-b", "127.0.0.1:9292")
	if err != nil {
		t.Fatal(err)
	}
	// slot-a has a half-prepared change; slot-b has a finalized one we must not touch.
	if _, err := a.begin("pa", RoleEX); err != nil {
		t.Fatal(err)
	}
	if _, err := b.begin("pb", RoleEX); err != nil {
		t.Fatal(err)
	}
	tb, _ := b.readTxn("pb")
	tb.Phase = PhaseFinalized
	if err := b.writeTxn(tb); err != nil {
		t.Fatal(err)
	}
	b.clearActive()
	bSnap, _ := os.ReadFile(b.txnPath("pb"))
	reports, err := RecoverAllAtBoot(ctx, n.Manager.Env)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]BootAction{}
	for _, r := range reports {
		got[r.Instance] = r.Action
	}
	if got[""] != BootNone || got["slot-a"] != BootRolledBack || got["slot-b"] != BootNone {
		raw, _ := json.Marshal(reports)
		t.Fatalf("reports: %s", raw)
	}
	if after, _ := os.ReadFile(b.txnPath("pb")); string(after) != string(bSnap) {
		t.Fatal("sibling instance state changed")
	}
	if a.activeID() != "" {
		t.Fatal("slot-a still locked")
	}
}
