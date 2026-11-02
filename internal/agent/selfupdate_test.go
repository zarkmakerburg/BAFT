package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/release"
)

// publishWithAgent signs a release that also carries baft-agent.
func (r *rig) publishWithAgent(version string) {
	r.t.Helper()
	dir := filepath.Join(r.relDir, version)
	os.MkdirAll(dir, 0o755)
	for _, n := range []string{"baft-linux-amd64", "baft-pair-linux-amd64", "baft-agent-linux-amd64"} {
		os.WriteFile(filepath.Join(dir, n), []byte(version+" "+n), 0o755)
	}
	if _, err := release.SignDir(release.SignInput{
		Dir: dir, Version: version, Commit: testCommit, CreatedAt: time.Now(),
		Provenance: release.Provenance{Builder: "test", GoVersion: "go"}, Key: r.relKey, Cert: r.cert,
	}); err != nil {
		r.t.Fatal(err)
	}
}

func sum(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func (r *rig) selfUpdating() {
	r.agent.cfg.AgentUnit = "baft-agent"
	os.WriteFile(filepath.Join(r.binDir, "baft-agent"), []byte("old agent"), 0o755)
}

func (r *rig) calls() string {
	r.sys.mu.Lock()
	defer r.sys.mu.Unlock()
	return strings.Join(r.sys.calls, "\n")
}

// update_baft on a self-updating agent installs the new baft-agent, keeps the
// old one, arms an external rollback guard BEFORE the swap, and restarts only
// after BCC has the result.
func TestSelfUpdateInstallsAgentBehindRollbackGuard(t *testing.T) {
	r := newRig(t)
	r.selfUpdating()
	r.publishWithAgent("v1.2.0")
	r.deploy("v1.2.0")
	res := r.runOne()
	if res.Status != "succeeded" || !strings.Contains(res.Detail, "baft-agent v1.2.0 installed") {
		t.Fatalf("result %+v", res)
	}
	if r.bin("baft") != "v1.2.0 baft-linux-amd64" || r.bin("baft-agent") != "v1.2.0 baft-agent-linux-amd64" || r.bin("baft-agent.prev") != "old agent" {
		t.Fatalf("binaries: baft=%q agent=%q prev=%q", r.bin("baft"), r.bin("baft-agent"), r.bin("baft-agent.prev"))
	}
	if r.jobStatus() != "succeeded" {
		t.Fatalf("BCC job status %q", r.jobStatus())
	}
	b, err := os.ReadFile(r.agent.agentUpdateMarkerPath())
	if err != nil {
		t.Fatal("no rollback marker")
	}
	var m agentUpdateMarker
	if json.Unmarshal(b, &m) != nil || m.NewSHA256 != sum("v1.2.0 baft-agent-linux-amd64") || m.Version != "v1.2.0" {
		t.Fatalf("marker %s", b)
	}
	calls := r.calls()
	if !strings.Contains(calls, "systemd-run --unit baft-agent-update-guard-") || !strings.Contains(calls, "--on-active 180") ||
		!strings.Contains(calls, "mv -f '"+filepath.Join(r.binDir, "baft-agent.prev")+"' '"+filepath.Join(r.binDir, "baft-agent")+"'") ||
		!strings.Contains(calls, "systemctl restart 'baft-agent'") {
		t.Fatalf("guard not armed as expected:\n%s", calls)
	}
	if !r.agent.RestartRequested() {
		t.Fatal("the agent must restart into the new binary once the result reached BCC")
	}
	if err := r.agent.restartIntoUpdate(context.Background()); err != nil || !strings.Contains(r.calls(), "restart --no-block baft-agent") {
		t.Fatalf("restart: %v\n%s", err, r.calls())
	}
}

// Only the new binary disarms the guard, and only after an authenticated poll.
func TestSelfUpdateConfirmedOnlyByTheNewAgent(t *testing.T) {
	r := newRig(t)
	r.selfUpdating()
	r.publishWithAgent("v1.2.0")
	r.deploy("v1.2.0")
	r.runOne()
	marker := r.agent.agentUpdateMarkerPath()

	old := r.newAgent(r.jobKey, r.agent.cfg.ReleaseRoot, r.agent.cfg.StateDir)
	old.selfDigest = func() (string, error) { return sum("old agent"), nil }
	if _, err := old.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("the previous agent disarmed the guard")
	}

	// The new binary, but BCC does not accept it (wrong token): still armed.
	refused := r.newAgent(r.jobKey, r.agent.cfg.ReleaseRoot, r.agent.cfg.StateDir)
	refused.cfg.Token = "not-the-agent-token"
	refused.selfDigest = func() (string, error) { return sum("v1.2.0 baft-agent-linux-amd64"), nil }
	if _, err := refused.RunOnce(context.Background()); err == nil {
		t.Fatal("a poll with the wrong token succeeded")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("the guard was disarmed without an authenticated poll")
	}

	fresh := r.newAgent(r.jobKey, r.agent.cfg.ReleaseRoot, r.agent.cfg.StateDir)
	fresh.selfDigest = func() (string, error) { return sum("v1.2.0 baft-agent-linux-amd64"), nil }
	if _, err := fresh.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("the new agent did not disarm the guard")
	}
	if v := fresh.SelfUpdateConfirmed(); v != "v1.2.0" {
		t.Fatalf("confirmed %q", v)
	}
}

// If the guard cannot be armed, the agent binary is not touched; baft is
// still updated and the result says why the agent stayed.
func TestSelfUpdateWithoutGuardLeavesAgentAlone(t *testing.T) {
	r := newRig(t)
	r.selfUpdating()
	r.sys.runErr = map[string]error{"systemd-run": errors.New("no systemd")}
	r.publishWithAgent("v1.2.0")
	r.deploy("v1.2.0")
	res := r.runOne()
	if res.Status != "succeeded" || !strings.Contains(res.Detail, "baft-agent unchanged: could not arm the rollback guard") {
		t.Fatalf("result %+v", res)
	}
	if r.bin("baft-agent") != "old agent" || r.bin("baft") != "v1.2.0 baft-linux-amd64" {
		t.Fatalf("agent=%q baft=%q", r.bin("baft-agent"), r.bin("baft"))
	}
	if _, err := os.Stat(r.agent.agentUpdateMarkerPath()); !os.IsNotExist(err) {
		t.Fatal("marker left behind")
	}
	if r.agent.RestartRequested() {
		t.Fatal("restart requested although the agent was not replaced")
	}
}

// A server whose tunnels BCC deployed as instances has no `baft` service:
// the update must not fail on restarting it.
func TestUpdateWithoutTunnelServiceSucceeds(t *testing.T) {
	r := newRig(t)
	r.sys.loadState = "not-found"
	r.publish("v1.1.0")
	r.deploy("v1.1.0")
	res := r.runOne()
	if res.Status != "succeeded" || !strings.Contains(res.Detail, "no baft service here") {
		t.Fatalf("result %+v", res)
	}
	if strings.Contains(r.calls(), "restart baft") {
		t.Fatalf("restarted a service that does not exist:\n%s", r.calls())
	}
	if st, err := release.ReadState(r.state); err != nil || st.Version != "v1.1.0" {
		t.Fatalf("release state %+v %v", st, err)
	}
}
