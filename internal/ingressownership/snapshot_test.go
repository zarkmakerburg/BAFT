package ingressownership

import (
	"errors"
	"testing"
)

func TestFenceSnapshotRestartNeverAdoptsActiveSocket(t *testing.T) {
	r := New()
	owner := proof("restart-owner", 4)
	if err := r.Claim("127.0.0.1:443", owner); err != nil {
		t.Fatal(err)
	}
	snap, err := r.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RecoverFences(snap)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.owners) != 0 || len(restored.active) != 0 {
		t.Fatal("recovered active ownership without host proof")
	}
	if err := restored.Claim("127.0.0.1:443", owner); !errors.Is(err, ErrStale) {
		t.Fatalf("replay admitted: %v", err)
	}
	if err := restored.Claim("127.0.0.1:443", proof("restart-owner", 5)); err != nil {
		t.Fatal(err)
	}
}
func TestFenceSnapshotRejectsCorruptAndMissingHighwater(t *testing.T) {
	r := New()
	owner := proof("restart-owner", 4)
	if err := r.Claim("127.0.0.1:443", owner); err != nil {
		t.Fatal(err)
	}
	snap, err := r.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	bad := snap
	bad.Generations = nil
	if _, err := RecoverFences(bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing generation: %v", err)
	}
	bad = snap
	bad.Active = append(append([]ActiveFence{}, snap.Active...), snap.Active[0])
	if _, err := RecoverFences(bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate port: %v", err)
	}
	bad = snap
	bad.Generations = append(append([]GenerationFence{}, snap.Generations...), snap.Generations[0])
	if _, err := RecoverFences(bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate generation: %v", err)
	}
}

func TestFenceSnapshotReleasedGenerationSurvivesRestart(t *testing.T) {
	r := New()
	old := proof("released", 2)
	if err := r.Claim("127.0.0.1:443", old); err != nil {
		t.Fatal(err)
	}
	if err := r.Release("127.0.0.1:443", old); err != nil {
		t.Fatal(err)
	}
	snap, err := r.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Active) != 0 || len(snap.Generations) != 1 {
		t.Fatalf("unexpected checkpoint: %+v", snap)
	}
	next, err := RecoverFences(snap)
	if err != nil {
		t.Fatal(err)
	}
	if err := next.Claim("127.0.0.1:443", old); !errors.Is(err, ErrStale) {
		t.Fatalf("released proof replay: %v", err)
	}
	if err := next.Claim("127.0.0.1:443", proof("released", 3)); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotRejectsInconsistentInternalOwnership(t *testing.T) {
	r := New()
	owner := proof("consistent", 1)
	if err := r.Claim("127.0.0.1:443", owner); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	delete(r.active, key(owner))
	r.mu.Unlock()
	if _, err := r.Snapshot(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("accepted inconsistent active index: %v", err)
	}
}

func TestSnapshotRejectsOrphanActiveIndex(t *testing.T) {
	r := New()
	owner := proof("orphan", 1)
	if err := r.Claim("127.0.0.1:443", owner); err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	r.active["foreign\x00"] = "443"
	r.mu.Unlock()
	if _, err := r.Snapshot(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("accepted orphan active index: %v", err)
	}
}
