//go:build unix

package agent

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestSeenJournalProcessHelper(t *testing.T) {
	dir := os.Getenv("BAFT_SEEN_HELPER_DIR")
	if dir == "" {
		return
	}
	a := &Agent{cfg: Config{StateDir: dir, Now: time.Now}, seen: map[string]time.Time{}}
	err := a.markSeen(os.Getenv("BAFT_SEEN_HELPER_JOB"))
	switch os.Getenv("BAFT_SEEN_HELPER_EXPECT") {
	case "claim":
		if err != nil {
			t.Fatal(err)
		}
	case "duplicate":
		if !errors.Is(err, errJobAlreadySeen) {
			t.Fatalf("duplicate admitted: %v", err)
		}
	case "locked":
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			t.Fatalf("locked journal admitted claim: %v", err)
		}
	default:
		t.Fatal("invalid helper expectation")
	}
}

func TestSeenJournalClaimsAreCrossProcessAndPreserveOtherIDs(t *testing.T) {
	dir := t.TempDir()
	child := func(id, expected string) {
		t.Helper()
		cmd := exec.Command(os.Args[0], "-test.run", "^TestSeenJournalProcessHelper$", "-test.count=1")
		cmd.Env = append(os.Environ(), "BAFT_SEEN_HELPER_DIR="+dir, "BAFT_SEEN_HELPER_JOB="+id, "BAFT_SEEN_HELPER_EXPECT="+expected)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("child %s/%s: %v %s", id, expected, err, out)
		}
	}
	child("first", "claim")
	child("second", "claim")
	child("first", "duplicate")
	unlock, err := lockSeenJournal(filepath.Join(dir, "seen-jobs.json.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	child("blocked", "locked")
	unlock()
	child("third", "claim")
	raw, err := os.ReadFile(filepath.Join(dir, "seen-jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	var seen map[string]time.Time
	if err := json.Unmarshal(raw, &seen); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 3 || seen["first"].IsZero() || seen["second"].IsZero() || seen["third"].IsZero() {
		t.Fatalf("process claims overwrote IDs: %s", raw)
	}
}

func TestSeenJournalLockRejectsSymlinkAndSharedFiles(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "public"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "target")
			if err := os.WriteFile(target, []byte("untouched"), 0600); err != nil {
				t.Fatal(err)
			}
			lock := filepath.Join(dir, "lock")
			switch kind {
			case "symlink":
				if err := os.Symlink(target, lock); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(target, lock); err != nil {
					t.Fatal(err)
				}
			case "public":
				if err := os.WriteFile(lock, nil, 0644); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(lock, 0644); err != nil {
					t.Fatal(err)
				}
			}
			if unlock, err := lockSeenJournal(lock); err == nil {
				unlock()
				t.Fatal("unsafe lock accepted")
			}
			raw, err := os.ReadFile(target)
			if err != nil || string(raw) != "untouched" {
				t.Fatalf("lock touched unrelated file: %q %v", raw, err)
			}
		})
	}
}

// A modified on-disk ledger must not let a formerly verified signed job be
// re-admitted after the process starts (or during a new Agent startup).
func TestSeenJournalRejectsUnsafeLedgerFiles(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "public"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			a := &Agent{cfg: Config{StateDir: dir, Now: time.Now}, seen: map[string]time.Time{}}
			target := filepath.Join(dir, "other-file")
			if err := os.WriteFile(target, []byte("{}"), 0600); err != nil {
				t.Fatal(err)
			}
			ledger := a.seenPath()
			switch kind {
			case "symlink":
				if err := os.Symlink(target, ledger); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(target, ledger); err != nil {
					t.Fatal(err)
				}
			case "public":
				if err := os.WriteFile(ledger, []byte("{}"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(ledger, 0644); err != nil {
					t.Fatal(err)
				}
			}
			if err := a.loadSeen(); err == nil {
				t.Fatal("unsafe ledger accepted at startup")
			}
			if err := a.markSeen("must-not-execute"); err == nil {
				t.Fatal("unsafe ledger admitted signed job")
			}
			if a.isSeen("must-not-execute") {
				t.Fatal("unsafe ledger marked job as seen")
			}
			if got, err := os.ReadFile(target); err != nil || string(got) != "{}" {
				t.Fatalf("unrelated target modified: %q %v", got, err)
			}
		})
	}
}

func TestSeenJournalRejectsUnsafeStateDirectory(t *testing.T) {
	for _, kind := range []string{"public", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "agent-state")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			state := dir
			if kind == "public" {
				if err := os.Chmod(dir, 0777); err != nil {
					t.Fatal(err)
				}
			} else {
				state = filepath.Join(root, "alias")
				if err := os.Symlink(dir, state); err != nil {
					t.Fatal(err)
				}
			}
			a := &Agent{cfg: Config{StateDir: state, Now: time.Now}, seen: map[string]time.Time{}}
			if err := a.loadSeen(); err == nil {
				t.Fatal("unsafe state directory accepted at startup")
			}
			if err := a.markSeen("must-not-execute"); err == nil {
				t.Fatal("unsafe state directory admitted job")
			}
			if a.isSeen("must-not-execute") {
				t.Fatal("job cached from unsafe directory")
			}
			if _, err := os.Stat(filepath.Join(dir, "seen-jobs.json")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unsafe state directory wrote ledger: %v", err)
			}
		})
	}
}

func TestMarkSeenRefusesCorruptDiskAfterStartup(t *testing.T) {
	for _, raw := range []string{"null", "{"} {
		t.Run(raw, func(t *testing.T) {
			dir := t.TempDir()
			a := &Agent{cfg: Config{StateDir: dir, Now: time.Now}, seen: map[string]time.Time{}}
			if err := os.WriteFile(a.seenPath(), []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			if err := a.markSeen("must-not-execute"); err == nil {
				t.Fatal("corrupt replay journal authorized a job")
			}
			after, err := os.ReadFile(a.seenPath())
			if err != nil || string(after) != raw || a.isSeen("must-not-execute") {
				t.Fatalf("corrupt journal overwritten: %q %v", after, err)
			}
		})
	}
}
