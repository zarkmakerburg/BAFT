//go:build unix

package ingressunit

import (
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

func stageFixture(t *testing.T, directory string) (Candidate, bool, error) {
	t.Helper()
	spec := candidateSpec()
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	return Stage(directory, spec, proof, "listener-1", "/usr/local/bin/baft", "baft")
}

func TestStageOwnsExactUnitAndRefusesReplacement(t *testing.T) {
	dir := t.TempDir()
	unit, created, err := stageFixture(t, dir)
	if err != nil || !created {
		t.Fatalf("stage: created=%t err=%v", created, err)
	}
	path := filepath.Join(dir, unit.Name)
	got, err := os.ReadFile(path)
	if err != nil || !MatchesOwnedBytes(unit, got) {
		t.Fatalf("staged bytes mismatch: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Sys().(*syscall.Stat_t).Nlink != 1 {
		t.Fatalf("staged unit has a second link: %v", err)
	}
	if again, created, err := stageFixture(t, dir); err != nil || created || again.SHA256 != unit.SHA256 {
		t.Fatalf("idempotent stage: created=%t err=%v", created, err)
	}
	if err := os.WriteFile(path, []byte("unrelated service"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, created, err := stageFixture(t, dir); err == nil || created {
		t.Fatalf("replaced unrelated unit: created=%t err=%v", created, err)
	}
	got, err = os.ReadFile(path)
	if err != nil || string(got) != "unrelated service" {
		t.Fatalf("unrelated unit was modified: %q %v", got, err)
	}
}

func TestStageConcurrentIdempotence(t *testing.T) {
	dir := t.TempDir()
	var wait sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < cap(results); i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, _, err := stageFixture(t, dir)
			results <- err
		}()
	}
	wait.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("concurrent stage: %v", err)
		}
	}
}

func TestStageRefusesUnsafeOwnershipAndDirectory(t *testing.T) {
	dir := t.TempDir()
	alias := filepath.Join(t.TempDir(), "symlink-dir")
	if err := os.Symlink(dir, alias); err != nil {
		t.Fatal(err)
	}
	if _, _, err := stageFixture(t, alias); err == nil {
		t.Fatal("accepted symlink unit directory")
	}
	unit, err := Render(candidateSpec(), mustProof(t), "listener-1", "/usr/local/bin/baft", "baft")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, unit.Name)
	if err := os.Symlink(filepath.Join(dir, "victim"), path); err != nil {
		t.Fatal(err)
	}
	if _, _, err := stageFixture(t, dir); err == nil {
		t.Fatal("accepted symlink unit")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, unit.Bytes, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := stageFixture(t, dir); err == nil {
		t.Fatal("accepted unit with wrong mode")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0777); err != nil {
		t.Fatal(err)
	}
	if _, _, err := stageFixture(t, dir); err == nil {
		t.Fatal("accepted world writable unit directory")
	}
}

func mustProof(t *testing.T) ingressmanifest.Proof {
	t.Helper()
	proof, err := ingressmanifest.Preview(candidateSpec())
	if err != nil {
		t.Fatal(err)
	}
	return proof
}
