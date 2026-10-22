//go:build unix

package ingressownership

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLABCheckpointPersistsGenerationWithoutAdoptingSocket(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	r := New()
	owner := proof("checkpoint", 7)
	if err := r.Claim("127.0.0.1:443", owner); err != nil {
		t.Fatal(err)
	}
	snap, err := r.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveCheckpoint(dir, snap); err != nil {
		t.Fatal(err)
	}
	restored, err := LoadCheckpoint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.owners) != 0 {
		t.Fatal("checkpoint adopted live socket")
	}
	if err := restored.Claim("127.0.0.1:443", owner); !errors.Is(err, ErrStale) {
		t.Fatalf("replayed checkpoint generation: %v", err)
	}
	if err := restored.Claim("127.0.0.1:443", proof("checkpoint", 8)); err != nil {
		t.Fatal(err)
	}
}
func TestLABCheckpointTamperFailsClosed(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	r := New()
	snap, err := r.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveCheckpoint(dir, snap); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "fences.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"snapshot":{"generations":[],"active":[]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCheckpoint(dir); !errors.Is(err, ErrInvalid) {
		t.Fatalf("tamper accepted: %v", err)
	}
}

func TestLABCheckpointRejectsSymlinkDestination(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "fences.json")); err != nil {
		t.Fatal(err)
	}
	snap, _ := New().Snapshot()
	if err := SaveCheckpoint(dir, snap); !errors.Is(err, ErrInvalid) {
		t.Fatalf("symlink accepted: %v", err)
	}
	content, err := os.ReadFile(outside)
	if err != nil || string(content) != "unchanged" {
		t.Fatalf("outside changed: %q %v", content, err)
	}
}
func TestLABCheckpointRejectsNoncanonicalJSON(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	snap, _ := New().Snapshot()
	if err := SaveCheckpoint(dir, snap); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "fences.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, ' '), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCheckpoint(dir); !errors.Is(err, ErrInvalid) {
		t.Fatalf("noncanonical checkpoint accepted: %v", err)
	}
}
