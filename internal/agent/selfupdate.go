package agent

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zarkmakerburg/baft/internal/release"
)

// Agent self-update (update_baft with Config.AgentUnit set).
//
// The agent cannot replace itself and then check that the replacement works:
// once it restarts, the process that would roll back is gone. So the swap is
// guarded from outside the agent. Before it restarts into the new binary, the
// agent asks systemd for a one-shot timer (systemd-run) that, after
// AgentUpdateGuard, puts the previous binary back and restarts the agent
// unless a marker file is gone by then. The new agent deletes the marker only
// after BCC has accepted it (its first authenticated poll) and only if it
// really is the new binary (its own SHA-256 matches the marker). The agent
// restarts only after BCC has the job's result, so the report is never lost.

const defaultAgentUpdateGuard = 3 * time.Minute

type agentUpdateMarker struct {
	Version   string    `json:"version"`
	Commit    string    `json:"commit"`
	NewSHA256 string    `json:"new_sha256"`
	Previous  string    `json:"previous"`
	GuardUnit string    `json:"guard_unit"`
	Started   time.Time `json:"started"`
}

func (a *Agent) agentUpdateMarkerPath() string {
	return filepath.Join(a.cfg.StateDir, "agent-update.json")
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// shellQuote quotes s for /bin/sh.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// agentSelfUpdateArtifact is the release file of this agent's own binary.
func (a *Agent) agentSelfUpdateArtifact() string { return "baft-agent-linux-" + a.cfg.Arch }

// stageAgentSelfUpdate installs the verified baft-agent from dir and arms the
// external rollback guard. On success the agent restarts into it after the
// job's result reached BCC. Any failure leaves the running agent's binary in
// place and returns why.
func (a *Agent) stageAgentSelfUpdate(ctx context.Context, dir string, verified release.Verified) (string, error) {
	src := filepath.Join(dir, a.agentSelfUpdateArtifact())
	target := filepath.Join(a.cfg.BinDir, "baft-agent")
	newSum, err := fileSHA256(src)
	if err != nil {
		return "", err
	}
	if cur, err := fileSHA256(target); err == nil && cur == newSum {
		return "baft-agent is already " + verified.Version, nil
	}
	staged := filepath.Join(a.cfg.BinDir, ".baft-agent.new")
	if err := copyFile(src, staged, 0o755); err != nil {
		return "", err
	}
	prev := target + ".prev"
	if err := copyFile(target, prev, 0o755); err != nil && !errors.Is(err, os.ErrNotExist) {
		os.Remove(staged)
		return "", err
	}
	guard := a.cfg.AgentUpdateGuard
	if guard <= 0 {
		guard = defaultAgentUpdateGuard
	}
	var rnd [4]byte
	_, _ = rand.Read(rnd[:])
	marker := agentUpdateMarker{
		Version: verified.Version, Commit: verified.Commit, NewSHA256: newSum, Previous: prev,
		GuardUnit: a.cfg.AgentUnit + "-update-guard-" + hex.EncodeToString(rnd[:]), Started: a.cfg.Now().UTC(),
	}
	b, _ := json.Marshal(marker)
	markerPath := a.agentUpdateMarkerPath()
	if err := writeFileAtomic(markerPath, b, 0o600); err != nil {
		os.Remove(staged)
		return "", err
	}
	undo := func() {
		_ = os.Remove(markerPath)
		_ = os.Remove(staged)
	}
	// The guard is armed before the swap: if arming fails, nothing changed.
	script := fmt.Sprintf("if [ -f %s ]; then mv -f %s %s && rm -f %s && systemctl restart %s; fi",
		shellQuote(markerPath), shellQuote(prev), shellQuote(target), shellQuote(markerPath), shellQuote(a.cfg.AgentUnit))
	if out, err := a.cfg.System.Run(ctx, "systemd-run", "--unit", marker.GuardUnit, "--collect", "--quiet",
		"--on-active", strconv.Itoa(int(guard/time.Second)), "--timer-property=AccuracySec=1s",
		"/bin/sh", "-c", script); err != nil {
		undo()
		return "", fmt.Errorf("could not arm the rollback guard: %v %s", err, strings.TrimSpace(out))
	}
	if err := os.Rename(staged, target); err != nil {
		// The guard finds the marker and puts back the binary that is
		// already in place; harmless. Remove the marker so it does nothing.
		undo()
		return "", err
	}
	a.mu.Lock()
	a.restartAfterAck = true
	a.mu.Unlock()
	return fmt.Sprintf("baft-agent %s installed: the agent restarts into it after this report and goes back to the previous agent by itself if the new one has not reconnected within %s", verified.Version, guard), nil
}

// selfUpdateNote replaces the agent binary after baft and baft-pair were
// updated and recorded, and says what happened to it. A failure here does not
// undo the baft update: the agent keeps running its current binary.
func (a *Agent) selfUpdateNote(ctx context.Context, dir string, verified release.Verified) string {
	if a.cfg.AgentUnit == "" {
		return "; baft-agent unchanged (self-update is off on this agent)"
	}
	note, err := a.stageAgentSelfUpdate(ctx, dir, verified)
	if err != nil {
		return "; baft-agent unchanged: " + err.Error()
	}
	return "; " + note
}

// writeFileAtomic writes b to path through a synced temporary file.
func writeFileAtomic(path string, b []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// RestartRequested reports that a self-update installed a new agent binary
// and every result has reached BCC, so the agent should restart into it.
func (a *Agent) RestartRequested() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.restartAfterAck && len(a.unacked) == 0
}

// restartIntoUpdate asks systemd to restart this agent's unit without
// waiting; systemd then stops this process.
func (a *Agent) restartIntoUpdate(ctx context.Context) error {
	_, err := a.cfg.System.Systemctl(ctx, "restart", "--no-block", a.cfg.AgentUnit)
	return err
}

// confirmAgentSelfUpdate runs after an authenticated poll succeeded. When
// this process is the newly installed agent, it disarms the rollback guard by
// removing the marker. The previous agent (still running while its report is
// retried) never matches the new digest, so it cannot disarm the guard.
func (a *Agent) confirmAgentSelfUpdate() {
	b, err := os.ReadFile(a.agentUpdateMarkerPath())
	if err != nil {
		return
	}
	var m agentUpdateMarker
	if json.Unmarshal(b, &m) != nil || m.NewSHA256 == "" {
		return
	}
	digest := a.selfDigest
	if digest == nil {
		digest = func() (string, error) { return fileSHA256("/proc/self/exe") }
	}
	got, err := digest()
	if err != nil || got != m.NewSHA256 {
		return
	}
	_ = os.Remove(a.agentUpdateMarkerPath())
	a.mu.Lock()
	a.selfUpdateConfirmed = m.Version
	a.mu.Unlock()
}

// SelfUpdateConfirmed returns the version this agent confirmed it is running
// after a self-update, once; "" otherwise.
func (a *Agent) SelfUpdateConfirmed() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	v := a.selfUpdateConfirmed
	a.selfUpdateConfirmed = ""
	return v
}
