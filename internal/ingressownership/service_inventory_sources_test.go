package ingressownership

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStoppedSystemdMultiRootInventoryAndIncompleteVeto(t *testing.T) {
	rootA := trustedInventoryFixture(t)
	rootB := trustedInventoryFixture(t)
	writeInventoryFixture(t, rootA, "foreign.socket", "[Socket]\nListenStream=443\n")
	writeInventoryFixture(t, rootB, "stopped.socket", "[Socket]\nListenStream=54619\n")
	got, err := InspectSystemdSocketSources([]string{rootA, rootB})
	if err != nil || !got.ScopedComplete || len(got.Claims) != 2 {
		t.Fatalf("multi-root scan failed: %+v %v", got, err)
	}
	for _, port := range []uint16{443, 54619} {
		if err := got.VetoKnownStoppedPort(port); !errors.Is(err, ErrConflict) {
			t.Fatalf("foreign stopped claim %d bypassed: %v", port, err)
		}
	}
	if err := got.VetoKnownStoppedPort(2053); err != nil {
		t.Fatalf("unclaimed port was incorrectly hardcoded blocked: %v", err)
	}
	if _, err := InspectSystemdSocketSources(nil); !errors.Is(err, ErrPersistentServiceInventoryIncomplete) {
		t.Fatalf("no-source inventory accepted: %v", err)
	}
	if _, err := InspectSystemdSocketSources([]string{rootA, rootA}); !errors.Is(err, ErrPersistentServiceInventoryIncomplete) {
		t.Fatalf("duplicate source accepted: %v", err)
	}
	if _, err := InspectSystemdSocketSources([]string{rootA, filepath.Join(rootB, "missing")}); !errors.Is(err, ErrPersistentServiceInventoryIncomplete) {
		t.Fatalf("missing vendor root accepted: %v", err)
	}
	writeInventoryFixture(t, rootB, "unknown.socket", "[Socket]\nListenStream=https\n")
	incomplete, err := InspectSystemdSocketSources([]string{rootA, rootB})
	if !errors.Is(err, ErrPersistentServiceInventoryIncomplete) || incomplete.ScopedComplete || len(incomplete.Claims) != 0 {
		t.Fatalf("partial root exposed as trusted: %+v %v", incomplete, err)
	}
	if err := incomplete.VetoKnownStoppedPort(443); !errors.Is(err, ErrPersistentServiceInventoryIncomplete) {
		t.Fatalf("incomplete observations claimed to know availability: %v", err)
	}
	if err := os.Remove(filepath.Join(rootB, "unknown.socket")); err != nil {
		t.Fatal(err)
	}
}
