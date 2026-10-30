package tunnelnode

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestBeginConcurrentManagersClaimOneTransaction(t *testing.T) {
	root := newNode(t)
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	env := root.Env
	env.Instance = "same-instance"
	// begin asks Now only after checking existing active/transaction state.
	// Hold both independent managers there to expose the check/write race.
	env.Now = func() time.Time {
		arrived <- struct{}{}
		<-release
		return time.Now()
	}
	type result struct {
		id  string
		err error
	}
	results := make(chan result, 2)
	for _, id := range []string{"first", "second"} {
		m, err := New(env)
		if err != nil {
			t.Fatal(err)
		}
		go func() { _, err := m.begin(id, RoleEX); results <- result{id, err} }()
	}
	for range 2 {
		select {
		case <-arrived:
		case <-time.After(5 * time.Second):
			close(release)
			t.Fatal("concurrent managers did not reach admission barrier")
		}
	}
	close(release)
	winner, successes := "", 0
	for range 2 {
		r := <-results
		if r.err == nil {
			winner = r.id
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent admission accepted %d transactions, want exactly one", successes)
	}
	m, err := New(root.Env)
	if err != nil {
		t.Fatal(err)
	}
	m, err = m.ForInstance("same-instance", "127.0.0.1:9321")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := m.checkedActiveID(); err != nil || got != winner {
		t.Fatalf("active claim %q differs from winner %q: %v", got, winner, err)
	}
	for _, id := range []string{"first", "second"} {
		_, err := os.Stat(filepath.Join(m.stage(id), "txn.json"))
		if id == winner && err != nil {
			t.Fatal(err)
		}
		if id != winner && !os.IsNotExist(err) {
			t.Fatalf("losing admission staged transaction: %v", err)
		}
	}
}

func TestActiveClaimAcrossProcesses(t *testing.T) {
	if dir := os.Getenv("BAFT_TEST_ACTIVE_CLAIM_DIR"); dir != "" {
		m, err := New(Env{ConfigDir: filepath.Join(dir, "etc"), StateDir: filepath.Join(dir, "state"),
			UnitDir: filepath.Join(dir, "units"), BaftBin: baftBin, PairBin: pairBin, System: &fakeHost{}})
		if err != nil {
			t.Fatal(err)
		}
		id := os.Getenv("BAFT_TEST_ACTIVE_CLAIM_ID")
		outcome := "won"
		if err := m.setActive(id); err != nil {
			if !errors.Is(err, os.ErrExist) {
				t.Fatal(err)
			}
			outcome = "lost"
		}
		if err := os.WriteFile(filepath.Join(dir, id+".result"), []byte(outcome), 0600); err != nil {
			t.Fatal(err)
		}
		return
	}
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	type processResult struct {
		id     string
		output []byte
		err    error
	}
	results := make(chan processResult, 2)
	for _, id := range []string{"first", "second"} {
		go func() {
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestActiveClaimAcrossProcesses$")
			cmd.Env = append(os.Environ(), "BAFT_TEST_ACTIVE_CLAIM_DIR="+dir, "BAFT_TEST_ACTIVE_CLAIM_ID="+id)
			out, err := cmd.CombinedOutput()
			results <- processResult{id, out, err}
		}()
	}
	winner, successes := "", 0
	for range 2 {
		r := <-results
		if r.err != nil {
			t.Errorf("claim process failed unexpectedly: %v %s", r.err, r.output)
			continue
		}
		outcome, err := os.ReadFile(filepath.Join(dir, r.id+".result"))
		if err != nil {
			t.Error(err)
			continue
		}
		switch string(outcome) {
		case "won":
			winner = r.id
			successes++
		case "lost":
		default:
			t.Errorf("invalid claim outcome %q", outcome)
		}
	}
	if successes != 1 {
		t.Fatalf("processes accepted %d claims", successes)
	}
	marker := filepath.Join(dir, "state", "tunnels", "active")
	got, err := os.ReadFile(marker)
	if err != nil || string(got) != winner+"\n" {
		t.Fatalf("process claim %q, winner %q: %v", got, winner, err)
	}
	info, err := os.Stat(marker)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("claim is not private: %v", err)
	}
}

func TestActiveClaimRefusesExistingAndPartialMarkers(t *testing.T) {
	for _, contents := range []string{"", "first", "first\n", "invalid/id\n"} {
		t.Run(contents, func(t *testing.T) {
			m := newNode(t).Manager
			if err := os.MkdirAll(filepath.Dir(m.activePath()), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(m.activePath(), []byte(contents), 0600); err != nil {
				t.Fatal(err)
			}
			if err := m.setActive("second"); !errors.Is(err, os.ErrExist) {
				t.Fatalf("existing marker not refused: %v", err)
			}
			got, err := os.ReadFile(m.activePath())
			if err != nil || string(got) != contents {
				t.Fatalf("claim overwritten: %q %v", got, err)
			}
		})
	}
	t.Run("symlink", func(t *testing.T) {
		m := newNode(t).Manager
		if err := os.MkdirAll(filepath.Dir(m.activePath()), 0700); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(t.TempDir(), "foreign")
		if err := os.WriteFile(target, []byte("foreign\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, m.activePath()); err != nil {
			t.Fatal(err)
		}
		if err := m.setActive("second"); !errors.Is(err, os.ErrExist) {
			t.Fatalf("symlink marker not refused: %v", err)
		}
		got, err := os.ReadFile(target)
		if err != nil || string(got) != "foreign\n" {
			t.Fatalf("symlink target changed: %q %v", got, err)
		}
	})
}

func TestBeginTerminalMarkerRequiresReconciliation(t *testing.T) {
	for _, phase := range []string{PhaseFinalized, PhaseRolledBack} {
		for _, retained := range []bool{false, true} {
			t.Run(phase+"/"+map[bool]string{false: "cleared", true: "retained"}[retained], func(t *testing.T) {
				m := newNode(t).Manager
				old := Txn{Version: 1, ID: "old", Role: RoleEX, Phase: phase}
				if err := m.writeTxn(old); err != nil {
					t.Fatal(err)
				}
				if retained {
					if err := m.setActive(old.ID); err != nil {
						t.Fatal(err)
					}
				}
				_, err := m.begin("next", RoleEX)
				if retained {
					if !errors.Is(err, os.ErrExist) {
						t.Fatalf("retained terminal claim accepted or overwritten: %v", err)
					}
					if got, err := m.checkedActiveID(); err != nil || got != old.ID {
						t.Fatalf("terminal owner changed: %q %v", got, err)
					}
					if _, err := os.Stat(m.stage("next")); !os.IsNotExist(err) {
						t.Fatalf("rejected admission created stage: %v", err)
					}
				} else if err != nil {
					t.Fatalf("completed transaction without active claim blocked next admission: %v", err)
				}
				got, err := m.readTxn(old.ID)
				if err != nil || got.Phase != phase {
					t.Fatalf("terminal transaction changed: %+v %v", got, err)
				}
			})
		}
	}
}
