//go:build unix

package ingressownership

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestDurableJournalCrashReplayAndRecovery(t *testing.T) {
	dir := t.TempDir()
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

func TestDurableJournalConcurrentWritersAndFences(t *testing.T) {
	dir := t.TempDir()
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

func TestDurableJournalRejectsCorruptionAndSymlink(t *testing.T) {
	dir := t.TempDir()
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
