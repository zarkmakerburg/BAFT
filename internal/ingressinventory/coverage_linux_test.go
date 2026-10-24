//go:build linux

package ingressinventory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func installedFixture(t *testing.T) (string, string, installedPolicyV1) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("protected installer/root-owned profile requires privileged isolated Linux LAB")
	}
	root, err := os.MkdirTemp("/root", "baft-ingress-coverage-lab-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if err = os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "installed-units")
	if err = os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	ns, err := namespaceIdentity()
	if err != nil {
		t.Fatal(err)
	}
	p := installedPolicyV1{SchemaVersion: 1, NodeID: "ir-lab-01", Platform: "linux", NetNS: ns, Transport: "dedicated", InventoryV1: ManagedCoverageVersion, Manager: "systemd-socket-v1", SourceRoots: []string{source}}
	return filepath.Join(root, "managed-node.json"), source, p
}
func writeProfileFixture(t *testing.T, file string, p installedPolicyV1) {
	t.Helper()
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(file, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestInstalledCoverageLABStopsForeignEvenWithoutSocket(t *testing.T) {
	policyFile, source, profile := installedFixture(t)
	if err := os.WriteFile(filepath.Join(source, "foreign.socket"), []byte("[Socket]\nListenStream=54619\n"), 0600); err != nil {
		t.Fatal(err)
	}
	writeProfileFixture(t, policyFile, profile)
	e, err := collectFromInstalledPath(context.Background(), policyFile, profile.NodeID, 54619)
	if !errors.Is(err, ErrClaimConflict) {
		t.Fatalf("stopped disabled foreign service was not protected: %v %+v", err, e)
	}
	if !e.SupportedEnvelope || e.CoverageComplete || len(e.Stopped) != 1 || len(e.Sources) != 1 || len(e.PolicySHA256) != 64 || e.PolicyInode == 0 {
		t.Fatalf("inert evidence lacked source/freshness identity or falsely claimed global completeness: %+v", e)
	}
	if e.Stopped[0].Port != 54619 || e.Sources[0].Inode == 0 || len(e.Sources[0].SHA256) != 64 {
		t.Fatalf("missing stopped-service evidence: %+v", e)
	}
	e, err = collectFromInstalledPath(context.Background(), policyFile, profile.NodeID, 2053)
	if !errors.Is(err, ErrCoverageIncomplete) || e.CoverageComplete || len(e.UnknownSources) == 0 {
		t.Fatalf("nonmatching port was made available despite unproven managers: %v %+v", err, e)
	}
}
func TestInstalledCoverageLABActiveKernelListenerAndUnknownManager(t *testing.T) {
	policyFile, _, profile := installedFixture(t)
	writeProfileFixture(t, policyFile, profile)
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := uint16(ln.Addr().(*net.TCPAddr).Port)
	e, err := collectFromInstalledPath(context.Background(), policyFile, profile.NodeID, port)
	if !errors.Is(err, ErrClaimConflict) {
		t.Fatalf("live socket not protected: %v %+v", err, e)
	}
	found := false
	for _, c := range e.ActiveTCP {
		if c.Port == port && c.State == "0A" && c.Inode != 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("kernel listening inode absent from observation: %+v", e.ActiveTCP)
	}
}
func TestInstalledCoverageLABUntrustedOrDriftedPolicyNeverTrusted(t *testing.T) {
	policyFile, _, profile := installedFixture(t)
	writeProfileFixture(t, policyFile, profile)
	tests := []struct {
		name        string
		breakPolicy func(t *testing.T)
	}{
		{"wrong-node", func(_ *testing.T) { profile.NodeID = "wrong-node"; writeProfileFixture(t, policyFile, profile) }},
		{"wrong-namespace", func(_ *testing.T) { profile.NetNS = "net:[0]"; writeProfileFixture(t, policyFile, profile) }},
		{"unknown-field", func(t *testing.T) {
			if err := os.WriteFile(policyFile, []byte(`{"schema_version":1,"unsupported":true}`), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{"writable-file", func(t *testing.T) {
			writeProfileFixture(t, policyFile, profile)
			if err := os.Chmod(policyFile, 0666); err != nil {
				t.Fatal(err)
			}
		}},
		{"symlink-file", func(t *testing.T) {
			writeProfileFixture(t, policyFile, profile)
			if err := os.Rename(policyFile, policyFile+".real"); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(policyFile+".real", policyFile); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Restore a valid installed root before each mutation.
			_ = os.Remove(policyFile)
			profile.NodeID = "ir-lab-01"
			ns, _ := namespaceIdentity()
			profile.NetNS = ns
			writeProfileFixture(t, policyFile, profile)
			tc.breakPolicy(t)
			e, err := collectFromInstalledPath(context.Background(), policyFile, "ir-lab-01", 54619)
			if !errors.Is(err, ErrCoverageIncomplete) || e.CoverageComplete || len(e.UnknownSources) == 0 {
				t.Fatalf("untrusted profile granted coverage: %v %+v", err, e)
			}
		})
	}
}
func TestInstalledCoverageLABSourceAndGenerationRefusal(t *testing.T) {
	policyFile, root, p := installedFixture(t)
	writeProfileFixture(t, policyFile, p)
	// A profile cannot assert a non-existent managed root is complete.
	p.SourceRoots = append(p.SourceRoots, filepath.Join(root, "missing"))
	writeProfileFixture(t, policyFile, p)
	e, err := collectFromInstalledPath(context.Background(), policyFile, p.NodeID, 54619)
	if !errors.Is(err, ErrCoverageIncomplete) || e.CoverageComplete {
		t.Fatalf("missing manager accepted: %v %+v", err, e)
	}
	p.SourceRoots = p.SourceRoots[:1]
	p.Manager = "generic-container-runtime"
	writeProfileFixture(t, policyFile, p)
	_, err = collectFromInstalledPath(context.Background(), policyFile, p.NodeID, 54619)
	if !errors.Is(err, ErrCoverageIncomplete) {
		t.Fatalf("unsupported manager accepted: %v", err)
	}
	p.Manager = "systemd-socket-v1"
	writeProfileFixture(t, policyFile, p)
	file := filepath.Join(root, "ambiguous.socket")
	if err := os.WriteFile(file, []byte("[Socket]\nListenStream=https\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = collectFromInstalledPath(context.Background(), policyFile, p.NodeID, 54619)
	if !errors.Is(err, ErrCoverageIncomplete) {
		t.Fatalf("unparseable stopped reservation accepted: %v", err)
	}
	_ = os.Remove(file)
	e, err = collectFromInstalledPath(context.Background(), policyFile, p.NodeID, 54619)
	if !errors.Is(err, ErrCoverageIncomplete) || e.CoverageComplete || len(e.UnknownSources) < 1 {
		t.Fatalf("unmeasured managed execution scope was promoted to complete coverage: %v %v", err, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e, err := collectFromInstalledPath(ctx, policyFile, p.NodeID, 54619); !errors.Is(err, ErrCoverageIncomplete) || e.CoverageComplete {
		t.Fatalf("cancellation accepted: %v %+v", err, e)
	}
	if e, err := collectFromInstalledPath(context.Background(), policyFile, p.NodeID, 0); !errors.Is(err, ErrCoverageIncomplete) || e.CoverageComplete {
		t.Fatalf("zero port accepted: %v %+v", err, e)
	}
}
func TestInstalledManagedPublicEntrypointIgnoresCallerRoot(t *testing.T) {
	// This API has no argument for a root, boolean, file path or injected
	// permit. Root-owned synthetic test policy cannot change its fixed path.
	if InstalledPolicyPath != "/etc/baft/ingress/managed-node.json" {
		t.Fatal("unexpected installed policy contract")
	}
	_ = fmt.Sprintf("%s %s", InstalledPolicyPath, time.Now())
	if !strings.HasPrefix(InstalledPolicyPath, "/etc/baft/") {
		t.Fatal("mutable policy directory")
	}
}

func TestInstalledCoverageLABSourceProvenanceRejectsNonRootOrWritableFiles(t *testing.T) {
	policyFile, source, profile := installedFixture(t)
	unit := filepath.Join(source, "foreign.socket")
	if err := os.WriteFile(unit, []byte("[Socket]\nListenStream=54619\n"), 0600); err != nil {
		t.Fatal(err)
	}
	writeProfileFixture(t, policyFile, profile)
	observe := func() (CoverageEvidence, error) {
		return collectFromInstalledPath(context.Background(), policyFile, profile.NodeID, 54619)
	}
	first, err := observe()
	if !errors.Is(err, ErrClaimConflict) || len(first.Sources) != 1 || len(first.Sources[0].Files) != 1 || len(first.Sources[0].Files[0].SHA256) != 64 {
		t.Fatalf("source content/inode was not independently observed: %+v %v", first, err)
	}
	before := first.Sources[0].SHA256
	if err := os.WriteFile(unit, []byte("[Socket]\nListenStream=54619\n# changed-byte-fingerprint\n"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := observe()
	if !errors.Is(err, ErrClaimConflict) || len(second.Sources) != 1 || before == second.Sources[0].SHA256 {
		t.Fatalf("changed effective stopped-source bytes were not fingerprinted: %+v %v", second, err)
	}
	if err := os.Chown(unit, 501, 501); err != nil {
		t.Fatal(err)
	}
	if e, err := observe(); !errors.Is(err, ErrCoverageIncomplete) || e.CoverageComplete {
		t.Fatalf("non-root-owned stopped service file accepted: %+v %v", e, err)
	}
	if err := os.Chown(unit, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(source, 501, 501); err != nil {
		t.Fatal(err)
	}
	if e, err := observe(); !errors.Is(err, ErrCoverageIncomplete) || e.CoverageComplete {
		t.Fatalf("non-root-owned source directory accepted: %+v %v", e, err)
	}
	if err := os.Chown(source, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(source, 0777); err != nil {
		t.Fatal(err)
	}
	if e, err := observe(); !errors.Is(err, ErrCoverageIncomplete) || e.CoverageComplete {
		t.Fatalf("writable installed service root accepted: %+v %v", e, err)
	}
}

// This is NOT the host/VPS path: tests explicitly launch under a disposable,
// loopback-only Linux network namespace with no other namespace occupants.
// Every executor/manager class exclusion is measured again after the scan.
func TestManagedLABMeasuredExecutionOnlyInPrivateNamespace(t *testing.T) {
	current, err := namespaceIdentity()
	if err != nil {
		t.Fatal(err)
	}
	initNS, err := os.Readlink("/proc/1/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	if current == initNS {
		t.Skip("requires disposable private Linux netns")
	}
	profilePath, _, profile := installedFixture(t)
	writeProfileFixture(t, profilePath, profile)
	e, err := collectFromInstalledPath(context.Background(), profilePath, profile.NodeID, 54619)
	if err != nil || !e.CoverageComplete || len(e.UnknownSources) != 0 ||
		e.LABExecution.OwnPID != os.Getpid() || e.LABExecution.CurrentNetNS != current ||
		len(e.LABExecution.Interfaces) != 1 || e.LABExecution.Interfaces[0] != "lo" {
		t.Fatalf("independently measured isolated LAB baseline did not pass: %+v err=%v", e, err)
	}
	// A second process running inside the SAME network namespace is a
	// measured unsupported execution source, not something that can be
	// waived by a profile field or a previously clean baseline.
	peer := exec.Command("/bin/sleep", "10")
	if err := peer.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = peer.Process.Kill(); _ = peer.Wait() }()
	if e, err := collectFromInstalledPath(context.Background(), profilePath, profile.NodeID, 54619); !errors.Is(err, ErrCoverageIncomplete) || e.CoverageComplete || len(e.UnknownSources) == 0 {
		t.Fatalf("concurrent same-netns process was silently accepted: %+v err=%v", e, err)
	}
	_ = peer.Process.Kill()
	_ = peer.Wait()
	ln, err := net.Listen("tcp4", "127.0.0.1:54619")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	e, err = collectFromInstalledPath(context.Background(), profilePath, profile.NodeID, 54619)
	if !errors.Is(err, ErrClaimConflict) || !e.CoverageComplete || len(e.ActiveTCP) == 0 {
		t.Fatalf("live foreign listener not vetoed under complete LAB observation: %+v err=%v", e, err)
	}
}

func TestInstalledPolicyRejectsAmbiguousWireKeys(t *testing.T) {
	p := installedPolicyV1{SchemaVersion: 1, NodeID: "ir-lab-01", Platform: "linux", NetNS: "net:[123]", Transport: "dedicated", InventoryV1: 1, Manager: "systemd-socket-v1", SourceRoots: []string{"/etc/systemd/system"}}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseInstalledPolicy(raw, p.NodeID, p.NetNS); err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{`"node_id":"other","node_id":"ir-lab-01"`, `"schema_version":1`, `"Node_ID":"ir-lab-01"`, `"source_roots":["/etc/systemd/system"]`} {
		t.Run(extra, func(t *testing.T) {
			ambiguous := append(append([]byte(nil), raw[:len(raw)-1]...), []byte(","+extra+"}")...)
			if _, err := parseInstalledPolicy(ambiguous, p.NodeID, p.NetNS); !errors.Is(err, ErrUnsupportedManagedNode) {
				t.Fatalf("ambiguous installed profile accepted: %s: %v", ambiguous, err)
			}
		})
	}
}
