//go:build unix

package tunnelnode

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type lifecycleRestartGate struct {
	System
	entered chan struct{}
	release chan struct{}
}

func (g *lifecycleRestartGate) Systemctl(ctx context.Context, args ...string) (string, error) {
	if len(args) > 0 && args[0] == "restart" {
		close(g.entered)
		select {
		case <-g.release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return g.System.Systemctl(ctx, args...)
}

func TestLifecycleCommitExcludesConcurrentRollback(t *testing.T) {
	p := newPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	code, err := p.ex.PrepareEX(ctx, "pair", p.exp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.ir.PrepareIR(ctx, "pair", code, p.irp); err != nil {
		t.Fatal(err)
	}
	gate := &lifecycleRestartGate{System: p.ir.System, entered: make(chan struct{}), release: make(chan struct{})}
	env := p.ir.Env
	env.System = gate
	committer, err := New(env)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := committer.CommitIR(ctx, "pair"); done <- err }()
	finished := false
	var releaseOnce sync.Once
	openGate := func() { releaseOnce.Do(func() { close(gate.release) }) }
	defer func() {
		openGate()
		if !finished {
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Error("commit did not stop after barrier release")
			}
		}
	}()
	select {
	case <-gate.entered:
	case <-ctx.Done():
		t.Fatal("commit did not reach restart")
	}
	_, rollbackErr := p.ir.Rollback(ctx, "pair")
	openGate()
	commitErr := <-done
	finished = true
	if err := commitErr; err != nil {
		t.Fatalf("commit: %v", err)
	}
	if !errors.Is(rollbackErr, ErrLifecycleBusy) {
		t.Fatalf("another Manager did not refuse busy lifecycle: %v", rollbackErr)
	}
	txn, err := p.ir.readTxn("pair")
	if err != nil || txn.Phase != PhaseCommitted {
		t.Fatalf("commit disagrees with durable phase: %+v %v", txn, err)
	}
	if _, err := os.Stat(p.ir.liveConfig()); err != nil {
		t.Fatalf("committed config missing: %v", err)
	}
	if _, err := p.ir.Rollback(ctx, "pair"); err != nil {
		t.Fatalf("rollback after commit completed: %v", err)
	}
}

func TestLifecycleRotationExcludesOtherManagers(t *testing.T) {
	r := newRotRig(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	env := r.ex.Env
	env.Now = func() time.Time {
		once.Do(func() { close(entered); <-release })
		return time.Now()
	}
	first, err := New(env)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := first.RotatePrepareEX(ctx, "first", "t1", r.epoch); done <- err }()
	finished := false
	var releaseOnce sync.Once
	openGate := func() { releaseOnce.Do(func() { close(release) }) }
	defer func() {
		openGate()
		if !finished {
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Error("rotation did not stop after barrier release")
			}
		}
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("rotation did not reach staging")
	}
	_, secondErr := r.ex.RotatePrepareEX(ctx, "second", "t1", r.epoch)
	_, rollbackErr := r.ex.Rollback(ctx, "t1")
	openGate()
	rotationErr := <-done
	finished = true
	if err := rotationErr; err != nil {
		t.Fatal(err)
	}
	for _, err := range []error{secondErr, rollbackErr} {
		if !errors.Is(err, ErrLifecycleBusy) {
			t.Fatalf("overlapping operation did not refuse rotation lock: %v", err)
		}
	}
	if _, err := os.Stat(r.ex.rotDir("second")); !os.IsNotExist(err) {
		t.Fatalf("second rotation staged: %v", err)
	}
	if got := r.ex.rotationInProgress(); got != "first" {
		t.Fatalf("rotation owner changed: %q", got)
	}
	if _, err := r.ex.RotateRollback(ctx, "first", true); err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleLockAcrossProcesses(t *testing.T) {
	if dir := os.Getenv("BAFT_TEST_LIFECYCLE_DIR"); dir != "" {
		m, err := New(Env{ConfigDir: filepath.Join(dir, "etc"), StateDir: filepath.Join(dir, "state"), UnitDir: filepath.Join(dir, "units"), BaftBin: baftBin, PairBin: pairBin, System: &fakeHost{}})
		if err != nil {
			t.Fatal(err)
		}
		unlock, err := m.lockLifecycle(context.Background())
		outcome := "busy"
		if err == nil {
			outcome = "acquired"
			unlock()
		} else if !errors.Is(err, ErrLifecycleBusy) {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "outcome"), []byte(outcome), 0600); err != nil {
			t.Fatal(err)
		}
		return
	}
	n := newNode(t)
	unlock, err := n.lockLifecycle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	held := true
	defer func() {
		if held {
			unlock()
		}
	}()
	lockPath := filepath.Join(n.StateDir, ".lifecycle.lock")
	before, err := os.Lstat(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	child := func(want string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLifecycleLockAcrossProcesses$")
		cmd.Env = append(os.Environ(), "BAFT_TEST_LIFECYCLE_DIR="+n.dir)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("child lock: %v %s", err, out)
		}
		got, err := os.ReadFile(filepath.Join(n.dir, "outcome"))
		if err != nil || string(got) != want {
			t.Fatalf("child lock outcome %q, want %s: %v", got, want, err)
		}
	}
	child("busy")
	unlock()
	held = false
	child("acquired")
	after, err := os.Lstat(lockPath)
	if err != nil || !os.SameFile(before, after) {
		t.Fatalf("lock inode replaced after release: %v", err)
	}
}

func TestLifecycleLockRejectsUnsafeFilesAndCancellation(t *testing.T) {
	for _, kind := range []string{"public", "contents", "symlink", "hardlink", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			m := newNode(t).Manager
			if err := os.MkdirAll(m.StateDir, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(m.StateDir, ".lifecycle.lock")
			target := filepath.Join(t.TempDir(), "foreign")
			foreign := "foreign"
			if kind == "hardlink" {
				foreign = ""
			}
			if err := os.WriteFile(target, []byte(foreign), 0600); err != nil {
				t.Fatal(err)
			}
			var err error
			switch kind {
			case "public":
				err = os.WriteFile(path, nil, 0644)
				if err == nil {
					err = os.Chmod(path, 0644)
				}
			case "contents":
				err = os.WriteFile(path, []byte("foreign"), 0600)
			case "symlink":
				err = os.Symlink(target, path)
			case "hardlink":
				err = os.Link(target, path)
			case "fifo":
				err = syscallMkfifo(path)
			}
			if err != nil {
				t.Fatal(err)
			}
			if unlock, err := m.lockLifecycle(context.Background()); err == nil {
				unlock()
				t.Fatal("unsafe lock accepted")
			}
			got, err := os.ReadFile(target)
			if err != nil || string(got) != foreign {
				t.Fatalf("foreign file changed: %q %v", got, err)
			}
		})
	}
	t.Run("cancelled", func(t *testing.T) {
		m := newNode(t).Manager
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := m.lockLifecycle(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled lock: %v", err)
		}
		if _, err := os.Stat(m.StateDir); !os.IsNotExist(err) {
			t.Fatalf("cancelled lock created state: %v", err)
		}
	})
	t.Run("writable-directory", func(t *testing.T) {
		m := newNode(t).Manager
		if err := os.MkdirAll(m.StateDir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(m.StateDir, 0777); err != nil {
			t.Fatal(err)
		}
		if unlock, err := m.lockLifecycle(context.Background()); err == nil {
			unlock()
			t.Fatal("writable directory accepted")
		}
	})
}

func TestLifecycleHeldLockFencesEveryMutationEntry(t *testing.T) {
	n := newNode(t)
	ctx := context.Background()
	unlock, err := n.lockLifecycle(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	m, err := New(n.Env)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]func() error{
		"prepare-ex":         func() error { _, e := m.PrepareEX(ctx, "new", ExParams{}); return e },
		"prepare-ir":         func() error { _, e := m.PrepareIR(ctx, "new", "", IRParams{}); return e },
		"commit-ex":          func() error { _, e := m.CommitEX(ctx, "new", ""); return e },
		"commit-ir":          func() error { _, e := m.CommitIR(ctx, "new"); return e },
		"finalize":           func() error { _, e := m.Finalize(ctx, "new"); return e },
		"rollback":           func() error { _, e := m.Rollback(ctx, "new"); return e },
		"retire":             func() error { _, e := m.Retire(ctx, "new", RetireExpectation{}); return e },
		"rotate-prepare-ex":  func() error { _, e := m.RotatePrepareEX(ctx, "rotation", "new", 1); return e },
		"rotate-trust-ir":    func() error { _, e := m.RotateTrustIR(ctx, "rotation", "new", 1, nil, ""); return e },
		"rotate-verify-ex":   func() error { _, e := m.RotateVerifyEX(ctx, "rotation"); return e },
		"rotate-verify-ir":   func() error { _, e := m.RotateVerifyIR(ctx, "rotation", nil, ""); return e },
		"rotate-activate-ex": func() error { _, e := m.RotateActivateEX(ctx, "rotation"); return e },
		"rotate-confirm-ir":  func() error { _, e := m.RotateConfirmIR(ctx, "rotation", ""); return e },
		"rotate-retire-ir":   func() error { _, e := m.RotateRetireIR(ctx, "rotation"); return e },
		"rotate-retire-ex":   func() error { _, e := m.RotateRetireEX(ctx, "rotation"); return e },
		"rotate-rollback":    func() error { _, e := m.RotateRollback(ctx, "rotation", true); return e },
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, ErrLifecycleBusy) {
				t.Fatalf("mutation did not refuse held lock: %v", err)
			}
		})
	}
	if len(n.host.calls) != 0 {
		t.Fatalf("busy mutations reached service manager: %v", n.host.calls)
	}
	for _, path := range []string{m.ConfigDir, m.UnitDir, filepath.Join(m.StateDir, "tunnels"), filepath.Join(m.StateDir, "rotations")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("busy mutation created state: %s %v", path, err)
		}
	}
}
