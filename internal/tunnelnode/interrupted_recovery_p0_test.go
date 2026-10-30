package tunnelnode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// crashingHost panics at the named systemctl verb, emulating the process
// dying (power loss / kill -9) after files were already switched but before the
// change was verified. Deferred unlocks still run, exactly as the kernel
// releases flocks when a real process dies; no rollback code gets to run.
type crashingHost struct {
	*fakeHost
	crashOn string
}

func (c *crashingHost) Systemctl(ctx context.Context, args ...string) (string, error) {
	if args[0] == c.crashOn {
		panic("simulated process death")
	}
	return c.fakeHost.Systemctl(ctx, args...)
}

func withSystem(env Env, sys System) Env {
	env.System = sys
	return env
}

// An interrupted change (process death mid-commit, then reboot) is recovered
// by a brand-new Manager over the same directories: the previous tunnel comes
// back byte for byte, the node is unlocked, and nothing this change did not
// create (foreign unit/config files) is touched.
func TestInterruptedCommitIsRecoveredAfterRestartWithoutTouchingForeignFiles(t *testing.T) {
	p := newPair(t)
	p.build(t, "first")
	ctx := context.Background()
	if _, err := p.ex.Finalize(ctx, "first"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ir.Finalize(ctx, "first"); err != nil {
		t.Fatal(err)
	}
	exBefore, _ := os.ReadFile(p.ex.liveConfig())
	markerBefore, _ := os.ReadFile(p.ex.markerPath())

	foreignUnit := filepath.Join(p.ex.UnitDir, "x-ui.service")
	foreignCfg := filepath.Join(p.ex.ConfigDir, "x-ui.json")
	for path, body := range map[string]string{foreignUnit: "[Service]\nExecStart=/usr/bin/x-ui\n", foreignCfg: `{"foreign":true}`} {
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
	env := p.ex.Manager.Env
	crashed, err := New(withSystem(env, &crashingHost{fakeHost: p.ex.host, crashOn: "restart"}))
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
	if now, _ := os.ReadFile(p.ex.liveConfig()); string(now) == string(exBefore) {
		t.Fatal("fixture: the interrupted change had not switched the live config yet")
	}

	// "Reboot": a fresh Manager and a fresh service manager over the same disk.
	rebooted, err := New(withSystem(env, &fakeHost{}))
	if err != nil {
		t.Fatal(err)
	}
	if rebooted.activeID() != "second" {
		t.Fatalf("restarted node lost track of the interrupted change: %q", rebooted.activeID())
	}
	msg, err := rebooted.Rollback(ctx, "second")
	if err != nil || msg != "rolled back" {
		t.Fatalf("post-reboot rollback: %q %v", msg, err)
	}
	if now, _ := os.ReadFile(rebooted.liveConfig()); string(now) != string(exBefore) {
		t.Fatal("previous tunnel config was not restored byte for byte")
	}
	if now, _ := os.ReadFile(rebooted.markerPath()); string(now) != string(markerBefore) {
		t.Fatal("previous ownership marker was not restored")
	}
	if rebooted.activeID() != "" {
		t.Fatal("node still locked by the interrupted change")
	}
	if b, err := os.ReadFile(foreignUnit); err != nil || !strings.Contains(string(b), "x-ui") {
		t.Fatalf("foreign unit was touched: %v", err)
	}
	if b, err := os.ReadFile(foreignCfg); err != nil || string(b) != `{"foreign":true}` {
		t.Fatalf("foreign config was touched: %v", err)
	}
}
