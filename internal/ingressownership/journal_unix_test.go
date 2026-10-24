//go:build unix

package ingressownership

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

func newJournalTestDir(t *testing.T) string {
	t.Helper()
	parent, err := filepath.Abs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(parent, "private-journal")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestDurableJournalCrashReplayAndRecovery(t *testing.T) {
	dir := newJournalTestDir(t)
	j, err := OpenDurableJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	record := MutationRecord{Port: 54619, Owner: proof("durable", 1), Phase: PhaseIntent}
	j.fault = func(point string) error {
		if point == "before_record" {
			return errors.New("injected crash before intent")
		}
		return nil
	}
	if err := j.Advance(record); err == nil {
		t.Fatal("missing pre-write crash")
	}
	if got, err := j.Recover(); err != nil || len(got.Records) != 0 {
		t.Fatalf("pre-write recovery: %+v %v", got, err)
	}
	j.fault = func(point string) error {
		if point == "after_record" {
			return errors.New("injected crash after fsync")
		}
		return nil
	}
	if err := j.Advance(record); err == nil {
		t.Fatal("missing post-write crash")
	}
	reopened, err := OpenDurableJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Recover()
	if err != nil || len(got.NeedsReconciliation) != 1 || got.NeedsReconciliation[0] != record {
		t.Fatalf("recovery adopted or lost intent: %+v %v", got, err)
	}
	if err := reopened.Advance(record); err != nil {
		t.Fatalf("idempotent replay did not repair head: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, journalFilename(1))); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.Recover(); err == nil {
		t.Fatal("accepted deleted journal tail below durable head")
	}
}

func TestDurableJournalRetiredGenerationReplayFence(t *testing.T) {
	dir := newJournalTestDir(t)
	j, err := OpenDurableJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	record := MutationRecord{Port: 54619, Owner: proof("durable-retired-replay", 1)}
	for _, phase := range []MutationPhase{PhaseIntent, PhaseListenerBound, PhaseFirewallInstalled, PhaseVerified, PhaseRetireIntent, PhaseRetired} {
		record.Phase = phase
		if err := j.Advance(record); err != nil {
			t.Fatalf("advance %s: %v", phase, err)
		}
	}
	record.Phase = PhaseIntent
	if err := j.Advance(record); !errors.Is(err, ErrConflict) {
		t.Fatalf("retired generation reopened: %v", err)
	}
	got, err := j.Recover()
	if err != nil || len(got.Records) != 1 || got.Records[0].Phase != PhaseRetired || len(got.NeedsReconciliation) != 0 {
		t.Fatalf("retired durable journal changed: %+v %v", got, err)
	}
	// A process restart must preserve both the retired-generation tombstone
	// and the ability to reserve this dynamically chosen port for a new owner.
	reopened, err := OpenDurableJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.Advance(record); !errors.Is(err, ErrConflict) {
		t.Fatalf("restarted journal resurrected retired generation: %v", err)
	}
	replacement := MutationRecord{Port: record.Port, Owner: proof("replacement-after-retirement", 1), Phase: PhaseIntent}
	if err := reopened.Advance(replacement); err != nil {
		t.Fatalf("restarted journal did not release retired port: %v", err)
	}
	if err := j.Advance(MutationRecord{Port: record.Port, Owner: proof("third-owner", 1), Phase: PhaseIntent}); !errors.Is(err, ErrConflict) {
		t.Fatalf("competing owner claimed port after replacement: %v", err)
	}
	got, err = reopened.Recover()
	if err != nil || len(got.Records) != 2 || len(got.NeedsReconciliation) != 1 || got.NeedsReconciliation[0] != replacement {
		t.Fatalf("restarted ownership/recovery not preserved: %+v %v", got, err)
	}
}

func TestDurableJournalConcurrentWritersAndFences(t *testing.T) {
	dir := newJournalTestDir(t)
	a, _ := OpenDurableJournal(dir)
	b, _ := OpenDurableJournal(dir)
	record := MutationRecord{Port: 54619, Owner: proof("durable", 1), Phase: PhaseIntent}
	var wait sync.WaitGroup
	results := make(chan error, 12)
	for i := 0; i < cap(results); i++ {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			if i%2 == 0 {
				results <- a.Advance(record)
			} else {
				results <- b.Advance(record)
			}
		}(i)
	}
	wait.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("same-intent concurrency: %v", err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, journalFilename(2))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("duplicate intent was persisted: %v", err)
	}
	if err := a.Advance(MutationRecord{Port: 54619, Owner: proof("other", 1), Phase: PhaseIntent}); !errors.Is(err, ErrConflict) {
		t.Fatalf("accepted competing port: %v", err)
	}
	if err := a.Advance(MutationRecord{Port: 54619, Owner: proof("durable", 2), Phase: PhaseIntent}); !errors.Is(err, ErrConflict) {
		t.Fatalf("accepted generation jump before retirement: %v", err)
	}
	for _, phase := range []MutationPhase{PhaseListenerBound, PhaseFirewallInstalled, PhaseVerified, PhaseRetireIntent, PhaseRetired} {
		record.Phase = phase
		if err := a.Advance(record); err != nil {
			t.Fatalf("phase %s: %v", phase, err)
		}
	}
	if err := b.Advance(MutationRecord{Port: 54619, Owner: proof("durable", 2), Phase: PhaseIntent}); err != nil {
		t.Fatalf("next generation: %v", err)
	}
	if err := b.Advance(record); !errors.Is(err, ErrStale) {
		t.Fatalf("accepted stale generation: %v", err)
	}
}

func TestDurableJournalProcessWriter(t *testing.T) {
	dir := os.Getenv("BAFT_JOURNAL_TEST_DIR")
	if dir == "" {
		return
	}
	j, err := OpenDurableJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Advance(MutationRecord{Port: 54619, Owner: proof("process-writer", 1), Phase: PhaseIntent}); err != nil {
		t.Fatal(err)
	}
}

func TestDurableJournalInterprocessLock(t *testing.T) {
	dir := newJournalTestDir(t)
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	const writers = 6
	results := make(chan error, writers)
	for i := 0; i < writers; i++ {
		go func() {
			cmd := exec.Command(binary, "-test.run=^TestDurableJournalProcessWriter$")
			cmd.Env = append(os.Environ(), "BAFT_JOURNAL_TEST_DIR="+dir)
			output, err := cmd.CombinedOutput()
			if err != nil {
				results <- errors.Join(err, errors.New(string(output)))
				return
			}
			results <- nil
		}()
	}
	for i := 0; i < writers; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, journalFilename(2))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("interprocess replay created duplicate record: %v", err)
	}
	j, err := OpenDurableJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := j.Recover()
	if err != nil || len(got.Records) != 1 || got.Records[0].Phase != PhaseIntent {
		t.Fatalf("interprocess recovery: %+v %v", got, err)
	}
}

func TestDurableJournalRejectsCorruptionAndSymlink(t *testing.T) {
	dir := newJournalTestDir(t)
	j, _ := OpenDurableJournal(dir)
	record := MutationRecord{Port: 54619, Owner: proof("durable", 1), Phase: PhaseIntent}
	if err := j.Advance(record); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, journalFilename(1))
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "head.json"), path); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Recover(); err == nil {
		t.Fatal("accepted symlink journal entry")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := j.Recover(); err == nil {
		t.Fatal("accepted truncated journal entry")
	}
}

func TestDurableJournalAnchorsDirectoryAcrossPathSwap(t *testing.T) {
	parent, err := filepath.Abs(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(parent, "journal")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(parent, "victim")
	if err := os.Mkdir(victim, 0700); err != nil {
		t.Fatal(err)
	}
	j, err := OpenDurableJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(parent, "moved")
	j.fault = func(point string) error {
		if point != "before_record" {
			return nil
		}
		if err := os.Rename(dir, moved); err != nil {
			return err
		}
		return os.Symlink(victim, dir)
	}
	if err := j.Advance(MutationRecord{Port: 54619, Owner: proof("path-swap", 1), Phase: PhaseIntent}); err == nil {
		t.Fatal("acknowledged a journal whose trusted path was replaced")
	}
	if _, err := os.Stat(filepath.Join(victim, journalFilename(1))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("wrote through replaced directory path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(moved, journalFilename(1))); err != nil {
		t.Fatalf("anchored write missing: %v", err)
	}
}

func TestDurableJournalRejectsDirectoryReplacementBetweenOperations(t *testing.T) {
	dir := newJournalTestDir(t)
	j, err := OpenDurableJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(dir, dir+"-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := j.Advance(MutationRecord{Port: 54619, Owner: proof("replaced", 1), Phase: PhaseIntent}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("accepted replacement journal directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, journalFilename(1))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("wrote into replacement directory: %v", err)
	}
}
