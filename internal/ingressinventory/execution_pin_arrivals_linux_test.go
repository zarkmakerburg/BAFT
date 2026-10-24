//go:build linux

package ingressinventory

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestManagedExecutionPinRefusesRealLateSourcesLAB(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires privileged isolated Linux LAB before inspecting host namespaces")
	}
	ns, err := namespaceIdentity()
	if err != nil {
		t.Fatal(err)
	}
	initNS, err := os.Readlink("/proc/1/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	if ns == initNS {
		t.Skip("requires isolated Linux LAB")
	}
	path, root, policy := installedFixture(t)
	writeProfileFixture(t, path, policy)
	ctx := context.Background()
	pin, err := pinInstalledManagedLABExecution(ctx, path, policy.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	unit := filepath.Join(root, "late-foreign.socket")
	if err := os.WriteFile(unit, []byte("[Socket]\nListenStream=54619\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := checkInstalledManagedLABAtEffect(ctx, path, policy.NodeID, pin); !errors.Is(err, ErrCoverageIncomplete) {
		t.Fatalf("new persistent service escaped source comparison: %v", err)
	}
	if err := os.Remove(unit); err != nil {
		t.Fatal(err)
	}
	peer := exec.Command("/bin/sleep", "10")
	if err := peer.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = peer.Process.Kill(); _ = peer.Wait() }()
	if err := checkInstalledManagedLABAtEffect(ctx, path, policy.NodeID, pin); !errors.Is(err, ErrCoverageIncomplete) {
		t.Fatalf("accidental late namespace process escaped comparison: %v", err)
	}
}
