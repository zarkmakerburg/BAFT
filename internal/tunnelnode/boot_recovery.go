package tunnelnode

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Boot recovery reconciles a tunnel transaction that a power loss, reboot or
// kill -9 interrupted. It is deterministic and driven only by on-disk
// evidence: the transaction journal (txn.json + the active marker), the
// ownership marker, and the SHA-256 of the live config, unit and marker
// against the digests the transaction recorded BEFORE it switched each file.
//
// Rules (each file is classified independently):
//
//	installed  live bytes == the digest this change recorded for it
//	previous   live bytes == the backup this change took (or the file is
//	           absent and the change recorded that it did not exist before)
//	foreign    anything else (an operator or another tool wrote it)
//	missing    absent although the change recorded that it existed before
//
//	prepared transaction ................ roll back (nothing live was switched;
//	                                      drops staging, restores a replaced PKI)
//	committed, any foreign/missing file . FENCED: nothing is touched
//	committed, all files installed ...... roll FORWARD: leave the change exactly
//	                                      as committed (BCC still decides health
//	                                      and finalize), make sure the unit is
//	                                      enabled; roll back instead if the
//	                                      binary is gone or the config no longer
//	                                      validates
//	committed, otherwise ................ roll back to the previous files
//
// Boot recovery never starts, stops or restarts a service: it runs before the
// services (systemd ordering) and the normal boot starts what is enabled. The
// only service-manager calls it can make are read-only queries, daemon-reload,
// enable and disable. It never touches a file it cannot prove it wrote.

// BootAction is the outcome for one instance.
type BootAction string

const (
	BootNone       BootAction = "none"        // nothing unfinished
	BootRolledBack BootAction = "rolled_back" // previous state restored
	BootForward    BootAction = "forward"     // committed change kept as is
	BootFenced     BootAction = "fenced"      // evidence insufficient or foreign: untouched
)

// BootReport describes what boot recovery decided for one instance.
type BootReport struct {
	Instance string            `json:"instance,omitempty"`
	TunnelID string            `json:"tunnel_id,omitempty"`
	Phase    string            `json:"phase,omitempty"`
	Action   BootAction        `json:"action"`
	Reason   string            `json:"reason"`
	Files    map[string]string `json:"files,omitempty"`
	// Skipped lists service-manager verbs boot recovery refused to run.
	Skipped []string `json:"skipped_service_verbs,omitempty"`
}

// bootSystem is the only System boot recovery ever uses: service start/stop
// verbs are refused (recorded, not executed) so recovery can run before the
// services without ever waiting on, or racing, systemd's own start jobs.
type bootSystem struct {
	System
	skipped []string
}

func (b *bootSystem) Systemctl(ctx context.Context, args ...string) (string, error) {
	if len(args) > 0 {
		switch args[0] {
		case "restart", "stop", "start", "reload", "try-restart", "reload-or-restart", "kill", "reset-failed", "mask", "unmask":
			b.skipped = append(b.skipped, strings.Join(args, " "))
			return "", nil
		}
	}
	return b.System.Systemctl(ctx, args...)
}

const (
	evInstalled = "installed"
	evPrevious  = "previous"
	evForeign   = "foreign"
	evMissing   = "missing"
)

// classifyFile compares one live file with the transaction's recorded digests.
func classifyFile(live, installed, backup string, had bool) string {
	b, err := os.ReadFile(live)
	if errors.Is(err, os.ErrNotExist) {
		if had {
			return evMissing
		}
		return evPrevious
	}
	if err != nil {
		return evForeign
	}
	sum := shaHex(b)
	if installed != "" && sum == installed {
		return evInstalled
	}
	if had {
		if bb, err := os.ReadFile(backup); err == nil && shaHex(bb) == sum {
			return evPrevious
		}
	}
	return evForeign
}

// RecoverAtBoot reconciles this manager's (instance's) unfinished transaction.
// It takes the cooperative lifecycle lock; a busy lock is an error and nothing
// is changed.
func (m *Manager) RecoverAtBoot(ctx context.Context) (BootReport, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rep := BootReport{Instance: m.Instance, Action: BootNone}
	if err := ctx.Err(); err != nil {
		return rep, err
	}
	if _, err := os.Lstat(m.StateDir); errors.Is(err, os.ErrNotExist) {
		rep.Reason = "no state directory"
		return rep, nil
	}
	unlock, err := m.lockLifecycle(ctx)
	if err != nil {
		return rep, err
	}
	defer unlock()
	boot := &bootSystem{System: m.System}
	saved := m.System
	m.System = boot
	defer func() { m.System = saved }()
	rep = m.recoverLocked(ctx)
	rep.Instance = m.Instance
	rep.Skipped = boot.skipped
	return rep, nil
}

func fenced(rep BootReport, why string, args ...any) BootReport {
	rep.Action, rep.Reason = BootFenced, fmt.Sprintf(why, args...)
	return rep
}

func (m *Manager) recoverLocked(ctx context.Context) BootReport {
	rep := BootReport{Action: BootNone}
	if rid := m.rotationInProgress(); rid != "" {
		return fenced(rep, "certificate rotation %s is in progress; boot recovery does not touch rotations", rid)
	}
	cur, err := m.checkedActiveID()
	if err != nil {
		return fenced(rep, "%v", err)
	}
	entries, err := os.ReadDir(filepath.Join(m.StateDir, "tunnels"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fenced(rep, "cannot inspect stored transactions: %v", err)
	}
	var unfinished []Txn
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if !idRe.MatchString(e.Name()) {
			continue
		}
		t, err := m.readTxn(e.Name())
		if err != nil {
			if errors.Is(err, os.ErrNotExist) && e.Name() != cur {
				continue // an empty stage nobody owns
			}
			if errors.Is(err, os.ErrNotExist) {
				continue // handled below as a claim without a transaction
			}
			return fenced(rep, "transaction %s is unreadable or corrupt: %v", e.Name(), err)
		}
		if t.Phase == PhasePrepared || t.Phase == PhaseCommitted {
			unfinished = append(unfinished, t)
		}
	}
	sort.Slice(unfinished, func(i, j int) bool { return unfinished[i].ID < unfinished[j].ID })
	if len(unfinished) > 1 {
		return fenced(rep, "%d unfinished transactions; refusing to guess which one owns the node", len(unfinished))
	}
	if len(unfinished) == 0 {
		return m.recoverIdle(cur, rep)
	}
	t := unfinished[0]
	rep.TunnelID, rep.Phase = t.ID, t.Phase
	if cur != "" && cur != t.ID {
		return fenced(rep, "active marker names %q but the unfinished transaction is %q", cur, t.ID)
	}
	switch t.Phase {
	case PhasePrepared:
		return m.recoverPrepared(ctx, t, rep)
	default:
		return m.recoverCommitted(ctx, t, cur, rep)
	}
}

// recoverIdle handles an active marker with no unfinished transaction.
func (m *Manager) recoverIdle(cur string, rep BootReport) BootReport {
	if cur == "" {
		rep.Reason = "no unfinished transaction"
		return rep
	}
	rep.TunnelID = cur
	t, err := m.readTxn(cur)
	switch {
	case err == nil && (t.Phase == PhaseFinalized || t.Phase == PhaseRolledBack):
		// The transaction reached its terminal state; only the final marker
		// removal was interrupted.
		rep.Phase = t.Phase
		if err := removeDurable(m.activePath()); err != nil {
			return fenced(rep, "cannot clear the stale active marker: %v", err)
		}
		rep.Action, rep.Reason = BootRolledBack, "cleared a stale active marker of a "+t.Phase+" transaction"
		return rep
	case err != nil && errors.Is(err, os.ErrNotExist):
		// begin() claims the marker, creates the stage, then writes txn.json.
		// Nothing live exists before that file. Only an empty/absent stage is
		// proof that the crash happened inside that window.
		ents, derr := os.ReadDir(m.stage(cur))
		if derr != nil && !errors.Is(derr, os.ErrNotExist) {
			return fenced(rep, "active marker %s without a transaction and an unreadable stage: %v", cur, derr)
		}
		if len(ents) != 0 {
			return fenced(rep, "active marker %s without a transaction but its stage holds files; manual reconciliation required", cur)
		}
		if err := removeDurable(m.activePath()); err != nil {
			return fenced(rep, "cannot clear the orphan active marker: %v", err)
		}
		_ = os.Remove(m.stage(cur))
		rep.Action, rep.Reason = BootRolledBack, "cleared an active marker claimed before any transaction state existed (nothing live was touched)"
		return rep
	default:
		return fenced(rep, "active marker %s does not match a recoverable transaction: %v", cur, err)
	}
}

func (m *Manager) recoverPrepared(ctx context.Context, t Txn, rep BootReport) BootReport {
	// prepared means no live config, unit or marker was switched. A crash in
	// the middle of PrepareEX can leave the replaced PKI aside without the
	// flag having been persisted; the aside directory is named after this
	// (validated) transaction ID, so its existence is the evidence.
	if _, err := os.Stat(m.pkiDir() + ".before-" + t.ID); err == nil {
		t.PKIReplaced = true
	}
	if t.ConfigWritten || t.UnitChanged {
		return fenced(rep, "prepared transaction claims live changes (config_written=%v unit_changed=%v)", t.ConfigWritten, t.UnitChanged)
	}
	if msg, err := m.rollbackTxn(ctx, t); err != nil {
		return fenced(rep, "rollback of the prepared transaction failed: %s", msg)
	}
	rep.Action, rep.Reason = BootRolledBack, "prepared transaction rolled back; nothing live had been switched"
	return rep
}

func (m *Manager) recoverCommitted(ctx context.Context, t Txn, cur string, rep BootReport) BootReport {
	if !t.ConfigWritten {
		return fenced(rep, "committed transaction does not record that live files were switched")
	}
	rep.Files = map[string]string{
		"config": classifyFile(m.liveConfig(), t.InstalledConfigSHA, m.backup(t.ID, "baft.yaml"), t.HadConfig),
		"unit":   classifyFile(m.unitPath(), t.InstalledUnitSHA, m.backup(t.ID, "unit"), t.HadUnit),
		"marker": classifyFile(m.markerPath(), t.InstalledMarkerSHA, m.backup(t.ID, "marker.json"), t.HadMarker),
	}
	installed := 0
	for name, state := range rep.Files {
		switch state {
		case evForeign, evMissing:
			return fenced(rep, "%s is %s: it is neither what this change wrote nor what it replaced; nothing was touched", name, state)
		case evInstalled:
			installed++
		}
	}
	if installed == 3 && cur == t.ID {
		if why := m.forwardBlocker(ctx); why != "" {
			if msg, err := m.rollbackTxn(ctx, t); err != nil {
				return fenced(rep, "cannot roll forward (%s) and rollback failed: %s", why, msg)
			}
			rep.Action, rep.Reason = BootRolledBack, "rolled back because the committed change cannot be kept: "+why
			return rep
		}
		if st, _ := m.System.Systemctl(ctx, "is-enabled", m.Service); st != "enabled" {
			if _, err := m.System.Systemctl(ctx, "enable", m.Service); err != nil {
				return fenced(rep, "committed change is complete but %s could not be enabled: %v", m.Service, err)
			}
		}
		_, _ = m.System.Systemctl(ctx, "daemon-reload")
		rep.Action = BootForward
		rep.Reason = "config, unit and ownership marker all match the digests recorded by the committed change; kept as committed (health and finalize remain BCC's decision)"
		return rep
	}
	if msg, err := m.rollbackTxn(ctx, t); err != nil {
		return fenced(rep, "rollback failed: %s", msg)
	}
	rep.Action = BootRolledBack
	rep.Reason = fmt.Sprintf("committed change was only partially applied (%d of 3 files switched) or has no active owner; previous state restored from backups", installed)
	return rep
}

// forwardBlocker returns why a fully installed change must not be kept.
func (m *Manager) forwardBlocker(ctx context.Context) string {
	st, err := os.Stat(m.BaftBin)
	if err != nil || !st.Mode().IsRegular() {
		return "service binary " + m.BaftBin + " is not present"
	}
	if out, err := m.System.Run(ctx, m.BaftBin, "config", "validate", "--file", m.liveConfig()); err != nil {
		return "live config does not validate: " + tailOf(out)
	}
	return ""
}

// RecoverAllAtBoot reconciles the singleton manager and every scoped instance
// that has state on this node. Instances are discovered only from
// StateDir/instances (real directories whose names are valid instance IDs).
func RecoverAllAtBoot(ctx context.Context, env Env) ([]BootReport, error) {
	base, err := New(env)
	if err != nil {
		return nil, err
	}
	var reports []BootReport
	var firstErr error
	note := func(rep BootReport, err error) {
		if err != nil {
			rep.Action, rep.Reason = BootFenced, "recovery could not run: "+err.Error()
			if firstErr == nil {
				firstErr = err
			}
		}
		reports = append(reports, rep)
	}
	note(base.RecoverAtBoot(ctx))
	ents, err := os.ReadDir(filepath.Join(base.StateDir, "instances"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return reports, errors.Join(firstErr, err)
	}
	for _, e := range ents {
		info, ierr := os.Lstat(filepath.Join(base.StateDir, "instances", e.Name()))
		if ierr != nil || !info.IsDir() || !instanceIDRe.MatchString(e.Name()) {
			continue
		}
		scoped := env
		scoped.Instance = e.Name()
		m, nerr := New(scoped)
		if nerr != nil {
			note(BootReport{Instance: e.Name()}, nerr)
			continue
		}
		m.MetricsListen = m.stagedMetricsListen()
		note(m.RecoverAtBoot(ctx))
	}
	return reports, firstErr
}

// stagedMetricsListen recovers an instance's validated metrics allocation
// from its staged pairing parameters so rolling back its claim releases the
// right port. Unknown keeps the default; ReleaseMetricsPort then releases
// nothing (a leaked claim is the fail-safe direction).
func (m *Manager) stagedMetricsListen() string {
	ents, err := os.ReadDir(filepath.Join(m.StateDir, "tunnels"))
	if err != nil {
		return m.MetricsListen
	}
	for _, e := range ents {
		if !e.IsDir() || !idRe.MatchString(e.Name()) {
			continue
		}
		var p struct {
			MetricsListen string `json:"MetricsListen"`
		}
		for _, f := range []string{"ex-params.json", "ir-params.json"} {
			if readJSON(filepath.Join(m.stage(e.Name()), f), &p) == nil && p.MetricsListen != "" {
				return p.MetricsListen
			}
		}
	}
	return m.MetricsListen
}
