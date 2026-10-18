package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/selfupdate"
	"github.com/zarkmakerburg/baft/internal/uninstall"
)

func uninstallServiceUnit(bin, cfg, stateDir string) string {
	return uninstall.InstallerServiceUnit("baft", bin, cfg, stateDir, false)
}

// ---- a host whose components are all one release behind ----

// defaultPlan is what a healthy install of a component plans for an update.
func defaultPlan(component string, changed bool) installerPlan {
	mk := func(steps ...planStep) installerPlan {
		p := installerPlan{State: installerHealthy, Why: "installed and running", Steps: steps}
		for _, s := range steps {
			if s.Action != "keep" && s.Action != "skip" {
				p.Changes++
			}
		}
		return p
	}
	act := func(a string) string {
		if changed {
			return a
		}
		return "keep"
	}
	switch component {
	case compBCC:
		return mk(planStep{"BCC binary", act("update"), "replace"}, planStep{"BCC bootstrap script", "keep", ""}, planStep{"BCC systemd unit", "keep", ""},
			planStep{"BCC service", act("restart"), "to pick up the binary"}, planStep{"BCC admin token", "keep", ""}, planStep{"BCC access", "keep", ""},
			planStep{"BCC job key", "keep", ""}, planStep{"release state", act("update"), "move forward"})
	case compAgent:
		return mk(planStep{"baft binary", act("update"), ""}, planStep{"baft-pair binary", "keep", ""}, planStep{"baft-agent binary", act("update"), ""},
			planStep{"agent unit", "keep", ""}, planStep{"agent token", "keep", ""}, planStep{"BCC job key", "keep", ""}, planStep{"release root pin", "keep", ""},
			planStep{"agent service", act("restart"), ""}, planStep{"release state", act("update"), ""})
	}
	return mk(planStep{"service user", "keep", ""}, planStep{"baft binary", act("update"), ""}, planStep{"baft-pair binary", "keep", ""},
		planStep{"release state", act("update"), ""}, planStep{"noise key", "keep", "exists; a rerun never regenerates it"}, planStep{"outer PKI", "keep", ""},
		planStep{"config", "keep", ""}, planStep{"pairing", "skip", "already paired"}, planStep{"systemd unit", "keep", ""}, planStep{"service", act("restart"), ""})
}

type fakeInstaller struct {
	rig       *applyRig
	runs      []installerSpec
	plans     map[string]installerPlan // overrides by mode (the plan made before the download)
	staged    map[string]installerPlan // overrides by mode (the plan after the download; default: the same as plans)
	failApply map[string]int           // exit code of the apply run by mode
	noUpgrade bool                     // apply "succeeds" but the binaries stay old
	applied   map[string]bool
	onApply   func(mode string) // called when an apply run finishes
}

func modeOf(args []string) string {
	if args[0] == "--role" {
		return args[1]
	}
	return args[0]
}

func componentOfMode(mode string) string {
	switch mode {
	case "--bcc-only":
		return compBCC
	case "--agent-only":
		return compAgent
	}
	return compTransport
}

func (f *fakeInstaller) run(ctx context.Context, s installerSpec) installerOutcome {
	f.runs = append(f.runs, s)
	// A real installer run dies with its context; the fake must notice a dead one too.
	if err := ctx.Err(); err != nil {
		f.rig.t.Errorf("the installer was started with a dead context (%v): %v", err, s.Args)
		return installerOutcome{Err: err}
	}
	mode := modeOf(s.Args)
	last := s.Args[len(s.Args)-1]
	switch last {
	case "--json":
		p, ok := f.plans[mode]
		if !ok {
			p = defaultPlan(componentOfMode(mode), !f.applied[mode])
		}
		if !contains(s.Args, "--plan") { // the plan after the download; a real run stops asking (exit 4)
			if sp, ok := f.staged[mode]; ok {
				p = sp
			}
			b, _ := json.Marshal(p)
			if p.Changes == 0 {
				return installerOutcome{Stdout: string(b)}
			}
			return installerOutcome{Stdout: string(b), ExitCode: exitInstallerAsks, Stderr: "re-run with --yes"}
		}
		b, _ := json.Marshal(p)
		return installerOutcome{Stdout: string(b)}
	case "--yes":
		if !s.Detach || s.LogPath == "" || s.Tee == nil {
			f.rig.t.Errorf("the apply run is not detached with a log: %+v", s)
		}
		if code := f.failApply[mode]; code != 0 {
			return installerOutcome{ExitCode: code, Stderr: "boom"}
		}
		f.applied[mode] = true
		if !f.noUpgrade {
			f.rig.upgrade(componentOfMode(mode))
		}
		if f.onApply != nil {
			f.onApply(mode)
		}
		return installerOutcome{}
	}
	f.rig.t.Fatalf("unexpected installer args %v", s.Args)
	return installerOutcome{}
}

// staged lists the modes whose plan was asked for after the download.
func (f *fakeInstaller) stagedModes() []string {
	var out []string
	for _, r := range f.runs {
		if r.Args[len(r.Args)-1] == "--json" && !contains(r.Args, "--plan") {
			out = append(out, modeOf(r.Args))
		}
	}
	return out
}

func (f *fakeInstaller) modes(suffix string) []string {
	var out []string
	for _, r := range f.runs {
		if r.Args[len(r.Args)-1] == suffix {
			out = append(out, modeOf(r.Args))
		}
	}
	return out
}

type applyRig struct {
	*invRig
	t       *testing.T
	h       *fakeHost
	inst    *fakeInstaller
	stub    *updateStub
	out     bytes.Buffer
	errOut  bytes.Buffer
	confirm func(string) bool
	asked   int
	backup  time.Time
	// stale names the services whose process still runs the previous binary
	// (the file on disk is new, the process was not restarted).
	stale map[string]bool
}

// runningDigest plays systemd + /proc: a restarted service runs the binary that
// is on disk, a stale one the previous release's.
func (r *applyRig) runningDigest(unit string) (string, error) {
	bin := map[string]string{r.layout.service: "baft", r.layout.agentUnit: "baft-agent", r.layout.bccService: "baft-bcc"}[unit]
	if bin == "" {
		return "", errors.New("no such service")
	}
	if r.stale[unit] {
		return hexSum(bin + "-v1"), nil
	}
	return sha256File(filepath.Join(r.dir, "bin", bin))
}

// newApplyRig installs a transport (IR) service, an agent and BCC, all one
// release behind v0.3.4.
func newApplyRig(t *testing.T) *applyRig {
	t.Helper()
	r := &applyRig{invRig: newInvRig(t), t: t, h: newFakeHost(t), backup: time.Now().Add(-time.Hour), stale: map[string]bool{}}
	cfgPath, _ := writeDialerConfigIn(t, filepath.Join(r.dir, "etc"), 0o600)
	bin := r.bin("baft", "baft-v1")
	r.bin("baft-pair", "pair-v1")
	r.unit(r.layout.service, uninstallServiceUnit(bin, cfgPath, r.layout.stateDir))
	r.agent()
	r.bcc(true)

	r.inst = &fakeInstaller{rig: r, failApply: map[string]int{}, applied: map[string]bool{}, staged: map[string]installerPlan{}}
	r.h.env.runInstaller = r.inst.run
	r.h.env.geteuid = func() int { return 0 }
	r.h.env.inventory = &inventoryHost{
		readFile: os.ReadFile, hashFile: sha256File,
		newestBackup:  func(string) (time.Time, error) { return r.backup, nil },
		listUnits:     listBaftUnits,
		runningDigest: r.runningDigest,
	}
	rep := reportAvailable
	rep.Artifacts = digests(map[string]string{"baft": "baft-v2", "baft-pair": "pair-v2", "baft-agent": "agent-v2", "baft-bcc": "bcc-v2"})
	rep.Artifacts[selfupdate.InstallerArtifact(runtime.GOARCH)] = hexSum(stubInstaller)
	r.stub = &updateStub{rep: rep}
	r.stub.install(r.h)
	r.confirm = func(string) bool { return true }
	return r
}

// upgrade swaps a component's binaries for the new release's.
func (r *applyRig) upgrade(component string) {
	switch component {
	case compBCC:
		r.bin("baft-bcc", "bcc-v2")
	case compAgent:
		r.bin("baft-agent", "agent-v2")
		r.bin("baft", "baft-v2")
		r.bin("baft-pair", "pair-v2")
	default:
		r.bin("baft", "baft-v2")
		r.bin("baft-pair", "pair-v2")
	}
}

func (r *applyRig) params() applyParams {
	return applyParams{
		layout: r.layout, yes: true, out: &r.out, errw: &r.errOut, env: r.h.env,
		confirm: func(q string) bool { r.asked++; return r.confirm(q) },
	}
}

func (r *applyRig) apply(mod func(*applyParams)) int {
	p := r.params()
	if mod != nil {
		mod(&p)
	}
	return applyUpdate(context.Background(), p)
}

func TestApplyUpdatesEveryComponentInOrderAndVerifies(t *testing.T) {
	r := newApplyRig(t)
	if code := r.apply(nil); code != 0 {
		t.Fatalf("code=%d\n%s%s", code, r.out.String(), r.errOut.String())
	}
	if got := strings.Join(r.inst.modes("--yes"), ","); got != "--bcc-only,ir,--agent-only" {
		t.Errorf("apply order = %s, want BCC, then the transport service, then the agent (they share baft and baft-pair)", got)
	}
	if got := len(r.inst.modes("--json")); got != 9 {
		t.Errorf("%d plan runs, want 3 before the download, 3 after it and 3 after the update", got)
	}
	if got := strings.Join(r.inst.stagedModes(), ","); got != "--bcc-only,ir,--agent-only" {
		t.Errorf("the plan after the download was asked for %q", got)
	}
	for _, s := range r.inst.runs {
		env := strings.Join(s.Env, "\n")
		for _, want := range []string{"BAFT_VERSION=v0.3.4", "BAFT_ALLOW_DOWNGRADE=0", "BAFT_NONINTERACTIVE=1", "BAFT_INSTALL_FROM=release", "BAFT_ROOT_PUB="} {
			if !strings.Contains(env, want) {
				t.Errorf("%v: environment lacks %s", s.Args, want)
			}
		}
		if s.Script != r.stub.rep.Installer && !strings.HasSuffix(s.Script, "baft-install-linux-"+runtime.GOARCH) {
			t.Errorf("the installer run is not the verified one: %s", s.Script)
		}
	}
	o := r.out.String()
	for _, w := range []string{"== bcc: running the signed installer (--bcc-only)", "tail -f ", "update-logs", "Updated to v0.3.4: bcc, transport, agent"} {
		if !strings.Contains(o, w) {
			t.Errorf("output lacks %q:\n%s", w, o)
		}
	}
	for _, s := range r.inst.runs {
		if s.Detach && !strings.HasPrefix(s.LogPath, filepath.Join(filepath.Dir(r.layout.releaseState), "update-logs")+string(filepath.Separator)) {
			t.Errorf("log outside the update-logs directory: %s", s.LogPath)
		}
	}
}

func TestTheInstallersOwnUpgradeAndCurrentStatesAreHealthy(t *testing.T) {
	// install.sh reports UPGRADE_AVAILABLE when the requested version differs
	// from the recorded one, CURRENT when nothing differs; both are refinements
	// of a healthy ALREADY_INSTALLED.
	for _, state := range []string{"UPGRADE_AVAILABLE", "CURRENT", "ALREADY_INSTALLED"} {
		r := newApplyRig(t)
		plans := map[string]installerPlan{}
		for _, m := range []string{"--bcc-only", "--agent-only", "ir"} {
			p := defaultPlan(componentOfMode(m), true)
			p.State = state
			plans[m] = p
		}
		r.inst.plans = plans
		if code := r.apply(func(p *applyParams) { p.planOnly = true }); code != 0 {
			t.Errorf("%s: code=%d\n%s", state, code, r.out.String())
		}
	}
	for _, state := range []string{"FRESH_INSTALL", "PARTIAL_INSTALL", "REPAIR_REQUIRED", "BROKEN_INSTALL", ""} {
		if len(vetPlan(installerPlan{State: state})) == 0 {
			t.Errorf("%q was accepted as a healthy install", state)
		}
	}
}

func TestEachInstallerGetsTheInputsOfThatInstall(t *testing.T) {
	r := newApplyRig(t)
	if code := r.apply(nil); code != 0 {
		t.Fatalf("code=%d\n%s", code, r.out.String())
	}
	env := map[string]map[string]string{}
	for _, s := range r.inst.runs {
		m := map[string]string{}
		for _, kv := range s.Env {
			k, v, _ := strings.Cut(kv, "=")
			m[k] = v
		}
		env[modeOf(s.Args)] = m
	}
	agent := env["--agent-only"]
	for k, want := range map[string]string{
		"BAFT_BCC_URL": "https://bcc.example.com", "BAFT_NODE_ID": "n1", "BAFT_BCC_JOB_KEY": "jobkey-public",
		"BAFT_AGENT_TOKEN_FILE": filepath.Join(r.dir, "agent", "token"), "BAFT_AGENT_DIR": filepath.Join(r.dir, "agent"),
		"BAFT_AGENT_INTERVAL": "30s", "BAFT_METRICS_LISTEN": "127.0.0.1:9191", "BAFT_SERVICE": r.layout.service,
	} {
		if agent[k] != want {
			t.Errorf("agent %s = %q, want %q", k, agent[k], want)
		}
	}
	if strings.Contains(strings.Join(r.inst.runs[1].Env, "\n"), "agent-token") {
		t.Error("the agent token itself is in the environment; only its file may be")
	}
	if ir := env["ir"]; ir["BAFT_SERVICE"] != r.layout.service || ir["BAFT_CONFIG_DIR"] != filepath.Join(r.dir, "etc") || ir["BAFT_BIN"] != filepath.Join(r.dir, "bin", "baft") {
		t.Errorf("transport env = %v", ir)
	}
	bcc := env["--bcc-only"]
	for k, want := range map[string]string{
		"BAFT_BCC_SERVICE": r.layout.bccService, "BAFT_BCC_BACKUP_DIR": filepath.Join(r.dir, "bccbak"),
		"BAFT_BCC_STATE_FILE": filepath.Join(r.dir, "bccvar", "state.db"), "BAFT_BCC_STATE_DIR": filepath.Join(r.dir, "bccvar"),
		"BAFT_BCC_ADMIN_TOKEN_FILE": filepath.Join(r.dir, "bccetc", "admin-token"), "BAFT_BCC_ACCESS_FILE": filepath.Join(r.dir, "bccetc", "access.json"),
		"BAFT_BCC_JOB_KEY_FILE": filepath.Join(r.dir, "bccetc", "job-key"), "BAFT_BCC_CONFIG_DIR": filepath.Join(r.dir, "bccetc"),
		"BAFT_BCC_BIN": filepath.Join(r.dir, "bin", "baft-bcc"),
	} {
		if bcc[k] != want {
			t.Errorf("bcc %s = %q, want %q", k, bcc[k], want)
		}
	}
}

func TestInstallerEnvIsBuiltFromScratch(t *testing.T) {
	t.Setenv("BAFT_ALLOW_DOWNGRADE", "1")
	t.Setenv("BAFT_PORT", "9999")
	t.Setenv("BAFT_ROOT_PUB", "attacker-key")
	t.Setenv("SSL_CERT_FILE", "/etc/ssl/lab.pem")
	t.Setenv("HTTPS_PROXY", "http://proxy:3128")
	t.Setenv("BAFT_RELEASE_URL", "https://rel.example/dl")
	env := installerEnv(menuCfg{releaseState: "/opt/baft/release-state.json", unitDir: "/etc/systemd/system"},
		updateTarget{Env: map[string]string{"BAFT_SERVICE": "baft"}}, "v0.3.4", "pinned-key", "/opt/baft/update-tmp/run-1")
	m := map[string]string{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		if _, dup := m[k]; dup {
			t.Errorf("%s set twice", k)
		}
		m[k] = v
	}
	for k, want := range map[string]string{
		"BAFT_ALLOW_DOWNGRADE": "0", "BAFT_ROOT_PUB": "pinned-key", "BAFT_VERSION": "v0.3.4", "BAFT_SERVICE": "baft",
		"BAFT_PREFIX": "/opt/baft", "BAFT_RELEASE_STATE": "/opt/baft/release-state.json",
		"SSL_CERT_FILE": "/etc/ssl/lab.pem", "HTTPS_PROXY": "http://proxy:3128", "BAFT_RELEASE_URL": "https://rel.example/dl",
	} {
		if m[k] != want {
			t.Errorf("%s = %q, want %q", k, m[k], want)
		}
	}
	if _, ok := m["BAFT_PORT"]; ok {
		t.Error("an unrelated BAFT_ variable leaked into the installer")
	}
}

func TestPlanOnlyChangesNothingAndNeedsNoRoot(t *testing.T) {
	r := newApplyRig(t)
	r.h.env.geteuid = func() int { return 1000 }
	code := r.apply(func(p *applyParams) { p.planOnly = true })
	if code != 0 {
		t.Fatalf("code=%d\n%s%s", code, r.out.String(), r.errOut.String())
	}
	if got := r.inst.modes("--yes"); len(got) != 0 {
		t.Fatalf("a preview applied %v", got)
	}
	o := r.out.String()
	for _, w := range []string{"[bcc] unit baft-bcc, installer --bcc-only", "[agent]", "[transport]", "BCC binary", "kept as they are:", "noise key", "This was a preview. Nothing was changed.",
		"will be replaced with the signed release's", "to run the new binary (a brief interruption of this service)", "move forward to v0.3.4 once the update verifies"} {
		if !strings.Contains(o, w) {
			t.Errorf("preview lacks %q:\n%s", w, o)
		}
	}
	if r.asked != 0 {
		t.Error("a preview asked for confirmation")
	}
}

func TestApplyNeedsRootExceptInTheMenu(t *testing.T) {
	r := newApplyRig(t)
	r.h.env.geteuid = func() int { return 1000 }
	if code := r.apply(nil); code != 1 || !strings.Contains(r.errOut.String(), "must run as root") || len(r.inst.runs) != 0 {
		t.Fatalf("code=%d err=%q runs=%d", code, r.errOut.String(), len(r.inst.runs))
	}
	r2 := newApplyRig(t)
	r2.h.env.geteuid = func() int { return 1000 }
	if code := r2.apply(func(p *applyParams) { p.degradeToPlan = true }); code != 0 || len(r2.inst.modes("--yes")) != 0 {
		t.Fatalf("code=%d applied=%v\n%s", code, r2.inst.modes("--yes"), r2.out.String())
	}
	if !strings.Contains(r2.out.String(), "shows the plan only") {
		t.Error("the degraded run does not say it is a plan only")
	}
}

func TestNothingRunsWithoutTheTypedConfirmation(t *testing.T) {
	r := newApplyRig(t)
	r.confirm = func(string) bool { return false }
	if code := r.apply(func(p *applyParams) { p.yes = false }); code != 1 {
		t.Fatalf("code=%d", code)
	}
	if r.asked != 1 || len(r.inst.modes("--yes")) != 0 || !strings.Contains(r.out.String(), "Not confirmed. Nothing was changed.") {
		t.Fatalf("asked=%d applied=%v\n%s", r.asked, r.inst.modes("--yes"), r.out.String())
	}
	r.confirm = func(string) bool { return true }
	if code := r.apply(func(p *applyParams) { p.yes = false }); code != 0 || len(r.inst.modes("--yes")) != 3 {
		t.Fatalf("code=%d applied=%v", code, r.inst.modes("--yes"))
	}
}

func TestTheCommandWantsATerminalOrYes(t *testing.T) {
	r := newApplyRig(t)
	var o, e bytes.Buffer
	code := runUpdateApply(context.Background(), nil, strings.NewReader("update\n"), &o, &e, r.h.env)
	if code != exitNeedsConfirmation || !strings.Contains(e.String(), "no terminal") || len(r.inst.runs) != 0 {
		t.Fatalf("code=%d err=%q runs=%d", code, e.String(), len(r.inst.runs))
	}
	o.Reset()
	args := []string{"--yes", "--unit-dir", r.layout.unitDir, "--service", r.layout.service, "--release-state", r.layout.releaseState}
	if code := runUpdateApply(context.Background(), args, strings.NewReader(""), &o, &e, r.h.env); code != 0 {
		t.Fatalf("--yes: code=%d\n%s%s", code, o.String(), e.String())
	}
}

func TestPlansThatTouchSecretsOrAreNotHealthyAreRefused(t *testing.T) {
	cases := []struct {
		name string
		mode string
		plan func(installerPlan) installerPlan
		want string
	}{
		{"a new noise key", "ir", func(p installerPlan) installerPlan { return withStep(p, "noise key", "create") }, `would create "noise key"`},
		{"config rewritten", "ir", func(p installerPlan) installerPlan { return withStep(p, "config", "replace") }, `would replace "config"`},
		{"pairing started", "ir", func(p installerPlan) installerPlan { return withStep(p, "pairing", "create") }, `would create "pairing"`},
		{"unit rewritten", "ir", func(p installerPlan) installerPlan { return withStep(p, "systemd unit", "update") }, `would update "systemd unit"`},
		{"BCC credentials rotated", "--bcc-only", func(p installerPlan) installerPlan { return withStep(p, "BCC access", "replace") }, `would replace "BCC access"`},
		{"panel unit rewritten", "--bcc-only", func(p installerPlan) installerPlan { return withStep(p, "BCC systemd unit", "update") }, `would update "BCC systemd unit"`},
		{"agent token replaced", "--agent-only", func(p installerPlan) installerPlan { return withStep(p, "agent token", "replace") }, `would replace "agent token"`},
		{"trust anchor changed", "--agent-only", func(p installerPlan) installerPlan { return withStep(p, "release root pin", "replace") }, `would replace "release root pin"`},
		{"packages installed", "ir", func(p installerPlan) installerPlan { return withStep(p, "packages", "create") }, "does not know"},
		{"a step from the future", "ir", func(p installerPlan) installerPlan { return withStep(p, "telemetry opt-in", "enable") }, "does not know"},
		{"not a healthy install", "ir", func(p installerPlan) installerPlan { p.State = "PARTIAL_INSTALL"; return p }, "PARTIAL_INSTALL"},
		{"a fresh install", "--bcc-only", func(p installerPlan) installerPlan { p.State = "FRESH_INSTALL"; return p }, "FRESH_INSTALL"},
	}
	for _, c := range cases {
		r := newApplyRig(t)
		r.inst.plans = map[string]installerPlan{c.mode: c.plan(defaultPlan(componentOfMode(c.mode), true))}
		if code := r.apply(nil); code != 1 {
			t.Errorf("%s: code=%d, want 1", c.name, code)
		}
		if got := r.inst.modes("--yes"); len(got) != 0 {
			t.Errorf("%s: applied %v although the plan was refused", c.name, got)
		}
		if o := r.out.String(); !strings.Contains(o, c.want) || !strings.Contains(o, "Nothing was changed") {
			t.Errorf("%s: output lacks %q:\n%s", c.name, c.want, o)
		}
	}
}

func withStep(p installerPlan, item, action string) installerPlan {
	steps := append([]planStep{}, p.Steps...)
	for i := range steps {
		if steps[i].Item == item {
			steps[i].Action = action
			p.Steps = steps
			return p
		}
	}
	p.Steps = append(steps, planStep{Item: item, Action: action})
	return p
}

func TestOnlyTheComponentsThatAreBehindAreUpdated(t *testing.T) {
	r := newApplyRig(t)
	// The shared release state said "current" and Core was updated, but BCC was not.
	r.upgrade(compTransport)
	r.upgrade(compAgent)
	if code := r.apply(nil); code != 0 {
		t.Fatalf("code=%d\n%s", code, r.out.String())
	}
	if got := strings.Join(r.inst.modes("--yes"), ","); got != "--bcc-only" {
		t.Fatalf("applied %s, want only the panel", got)
	}
	if o := r.out.String(); strings.Contains(o, "[transport]") || strings.Contains(o, "[agent]") {
		t.Errorf("components that are current are in the plan:\n%s", o)
	}
}

func TestEverythingCurrentMeansNothingToApply(t *testing.T) {
	r := newApplyRig(t)
	for _, c := range []string{compTransport, compAgent, compBCC} {
		r.upgrade(c)
	}
	if code := r.apply(nil); code != 0 || len(r.inst.runs) != 0 || !strings.Contains(r.out.String(), "Nothing to apply") {
		t.Fatalf("code=%d runs=%d\n%s", code, len(r.inst.runs), r.out.String())
	}
}

func TestOnlySelectsComponents(t *testing.T) {
	r := newApplyRig(t)
	if code := r.apply(func(p *applyParams) { p.only = map[string]bool{compBCC: true} }); code != 0 {
		t.Fatalf("code=%d\n%s", code, r.out.String())
	}
	if got := strings.Join(r.inst.modes("--yes"), ","); got != "--bcc-only" {
		t.Fatalf("applied %s", got)
	}
	r1 := newApplyRig(t)
	if code := r1.apply(func(p *applyParams) { p.only = map[string]bool{compTransport: true} }); code != 0 {
		t.Fatalf("code=%d\n%s", code, r1.out.String())
	}
	if got := strings.Join(r1.inst.modes("--yes"), ","); got != "ir" {
		t.Fatalf("applied %s", got)
	}
	r2 := newApplyRig(t)
	if err := os.Remove(filepath.Join(r2.layout.unitDir, "baft-bcc.service")); err != nil {
		t.Fatal(err)
	}
	if code := r2.apply(func(p *applyParams) { p.only = map[string]bool{compBCC: true} }); code != 1 || !strings.Contains(r2.out.String(), "--only bcc: that component is not installed here") || len(r2.inst.runs) != 0 {
		t.Fatalf("code=%d runs=%d\n%s", code, len(r2.inst.runs), r2.out.String())
	}
}

func TestABinaryIsNeverReplacedUnderAServiceOutsideTheRun(t *testing.T) {
	// The agent installer replaces baft and baft-pair, which baft.service runs;
	// an installer restarts only its own service, so the tunnel would keep the
	// old process and switch to the new code at its next restart, unannounced.
	r := newApplyRig(t)
	if code := r.apply(func(p *applyParams) { p.only = map[string]bool{compAgent: true} }); code != 1 || len(r.inst.modes("--yes")) != 0 {
		t.Fatalf("code=%d applied=%v\n%s", code, r.inst.modes("--yes"), r.out.String())
	}
	if o := r.out.String(); !strings.Contains(o, "baft runs ") || !strings.Contains(o, "is not part of this run") {
		t.Errorf("the refusal does not say why:\n%s", o)
	}

	// The same for a service the BCC tunnel builder added next to baft.service.
	r2 := newApplyRig(t)
	bin := filepath.Join(r2.dir, "bin", "baft")
	extra := uninstallServiceUnit(bin, filepath.Join(r2.dir, "etc", "instances", "t1", "baft.yaml"), filepath.Join(r2.dir, "var", "t1"))
	r2.unit("baft-t1", extra)
	if code := r2.apply(nil); code != 1 || len(r2.inst.modes("--yes")) != 0 {
		t.Fatalf("code=%d applied=%v\n%s", code, r2.inst.modes("--yes"), r2.out.String())
	}
	if o := r2.out.String(); !strings.Contains(o, "baft-t1 runs ") {
		t.Errorf("the unlisted service is not named:\n%s", o)
	}
	// Updating only BCC touches neither binary and is fine.
	r3 := newApplyRig(t)
	r3.unit("baft-t1", extra)
	if code := r3.apply(func(p *applyParams) { p.only = map[string]bool{compBCC: true} }); code != 0 {
		t.Fatalf("code=%d\n%s", code, r3.out.String())
	}
}

func TestTheAgentIsNotRunAgainWhenTheTransportPassAlreadyUpdatedItsFiles(t *testing.T) {
	r := newApplyRig(t)
	r.bin("baft-agent", "agent-v2") // only baft and baft-pair, shared with the transport, are behind
	r.inst.applied["--agent-only"] = true // so the installer's own plan for the agent has nothing left, as the real one would
	if code := r.apply(nil); code != 0 {
		t.Fatalf("code=%d\n%s", code, r.out.String())
	}
	if got := strings.Join(r.inst.modes("--yes"), ","); got != "--bcc-only,ir" {
		t.Fatalf("applied %s", got)
	}
	if o := r.out.String(); !strings.Contains(o, "agent: already current") || !strings.Contains(o, "Updated to v0.3.4: bcc, transport, agent") {
		t.Errorf("output:\n%s", o)
	}
}

func TestAServiceStillRunningTheOldProcessFailsTheUpdate(t *testing.T) {
	r := newApplyRig(t)
	r.stale[r.layout.service] = true
	if code := r.apply(nil); code != 1 {
		t.Fatalf("code=%d\n%s", code, r.out.String())
	}
	o := r.out.String()
	if !strings.Contains(o, "WARNING: "+r.layout.service+" is still running the previous baft") || strings.Contains(o, "Updated to v0.3.4") {
		t.Errorf("output:\n%s", o)
	}
}

func TestAServiceThatCannotBeInspectedIsSaidSoNotCountedAsVerified(t *testing.T) {
	r := newApplyRig(t)
	r.h.env.inventory.runningDigest = func(string) (string, error) { return "", errors.New("no systemd here") }
	if code := r.apply(nil); code != 0 {
		t.Fatalf("code=%d\n%s", code, r.out.String())
	}
	if o := r.out.String(); !strings.Contains(o, "NOTE: could not check which binary") || !strings.Contains(o, "no systemd here") {
		t.Errorf("output:\n%s", o)
	}
}

func TestAFailureStopsTheRunBeforeTheNextComponent(t *testing.T) {
	r := newApplyRig(t)
	r.inst.failApply["ir"] = 1
	if code := r.apply(nil); code != 1 {
		t.Fatalf("code=%d", code)
	}
	if got := strings.Join(r.inst.modes("--yes"), ","); got != "--bcc-only,ir" {
		t.Fatalf("applied %s: the agent must not be touched after a failure", got)
	}
	o := r.out.String()
	for _, w := range []string{"transport FAILED: exit 1: boom", "rolls back its own run", "Updated: bcc. Not started: agent."} {
		if !strings.Contains(o, w) {
			t.Errorf("output lacks %q:\n%s", w, o)
		}
	}
	if strings.Contains(o, "Updated to v0.3.4") {
		t.Error("a failed run claims success")
	}
}

func TestAnInterruptStopsBeforeTheNextComponentAndFalseAlarmsDoNotFollow(t *testing.T) {
	r := newApplyRig(t)
	ctx, cancel := context.WithCancel(context.Background())
	r.inst.onApply = func(mode string) {
		if mode == "--bcc-only" {
			cancel() // Ctrl-C while BCC is being updated; its installer still finishes
		}
	}
	p := r.params()
	if code := applyUpdate(ctx, p); code != 1 {
		t.Fatalf("code=%d\n%s", code, r.out.String())
	}
	if got := strings.Join(r.inst.modes("--yes"), ","); got != "--bcc-only" {
		t.Fatalf("applied %s: nothing may start after the interrupt", got)
	}
	o := r.out.String()
	if !strings.Contains(o, "Interrupted before the next component") || !strings.Contains(o, "Updated: bcc. Not started: transport, agent.") {
		t.Errorf("output:\n%s", o)
	}
	if strings.Contains(o, "still has changes pending") {
		t.Errorf("the interrupt produced a false alarm:\n%s", o)
	}
}

func TestAnUpdateThatDoesNotConvergeIsReported(t *testing.T) {
	r := newApplyRig(t)
	r.inst.noUpgrade = true // the installer exits 0 but the binaries are still the old ones
	if code := r.apply(nil); code != 1 {
		t.Fatalf("code=%d", code)
	}
	o := r.out.String()
	if !strings.Contains(o, "does not yet run the signed release's binaries") || strings.Contains(o, "Updated to v0.3.4") {
		t.Fatalf("output:\n%s", o)
	}
}

func TestTheInstallerIsReverifiedRightBeforeItRuns(t *testing.T) {
	r := newApplyRig(t)
	h := *r.h.env.inventory
	calls := 0
	h.hashFile = func(p string) (string, error) {
		calls++
		if strings.HasSuffix(p, "baft-install-linux-"+runtime.GOARCH) {
			return "00ff", nil // swapped after verification
		}
		return sha256File(p)
	}
	r.h.env.inventory = &h
	if code := r.apply(nil); code != 1 {
		t.Fatalf("code=%d", code)
	}
	if got := r.inst.modes("--yes"); len(got) != 0 {
		t.Fatalf("a tampered installer ran for %v", got)
	}
	if !strings.Contains(r.errOut.String(), "changed on disk") {
		t.Errorf("stderr = %q", r.errOut.String())
	}
}

func TestBCCIsOnlyUpdatedWithARecentBackup(t *testing.T) {
	for _, c := range []struct {
		name   string
		mod    func(*applyRig)
		skip   bool
		wantOK bool
		want   string
	}{
		{"fresh backup", func(*applyRig) {}, false, true, ""},
		{"stale backup", func(r *applyRig) { r.backup = time.Now().Add(-48 * time.Hour) }, false, false, "48h0m0s old"},
		{"stale but skipped on purpose", func(r *applyRig) { r.backup = time.Now().Add(-48 * time.Hour) }, true, true, ""},
		{"no backup at all", func(r *applyRig) {
			h := *r.h.env.inventory
			h.newestBackup = func(string) (time.Time, error) { return time.Time{}, errors.New("no backup files") }
			r.h.env.inventory = &h
		}, false, false, "no BCC backup found"},
	} {
		r := newApplyRig(t)
		c.mod(r)
		code := r.apply(func(p *applyParams) { p.skipBackupCheck = c.skip })
		if (code == 0) != c.wantOK {
			t.Errorf("%s: code=%d\n%s", c.name, code, r.out.String())
		}
		if !c.wantOK && (!strings.Contains(r.out.String(), c.want) || len(r.inst.modes("--yes")) != 0) {
			t.Errorf("%s: output lacks %q or something was applied:\n%s", c.name, c.want, r.out.String())
		}
	}
}

func TestAnUnrecognisedOrUnrecoverableComponentBlocksEverything(t *testing.T) {
	r := newApplyRig(t)
	r.unit(r.layout.agentUnit, "[Service]\nExecStart=/usr/local/bin/baft-agent --hand-made\n")
	if code := r.apply(nil); code != 1 || len(r.inst.runs) != 0 || !strings.Contains(r.out.String(), "agent (baft-agent)") {
		t.Fatalf("code=%d runs=%d\n%s", code, len(r.inst.runs), r.out.String())
	}

	r2 := newApplyRig(t)
	if err := os.Remove(filepath.Join(r2.dir, "agent", "bcc-job.pub")); err != nil {
		t.Fatal(err)
	}
	if code := r2.apply(nil); code != 1 || len(r2.inst.modes("--yes")) != 0 || !strings.Contains(r2.out.String(), "cannot read the pinned BCC job key") {
		t.Fatalf("code=%d\n%s", code, r2.out.String())
	}

	r3 := newApplyRig(t)
	r3.layout.releaseState = filepath.Join(r3.dir, "elsewhere", "release-state.json")
	if code := r3.apply(nil); code != 1 || !strings.Contains(r3.out.String(), "it records its release in") {
		t.Fatalf("code=%d\n%s", code, r3.out.String())
	}
}

func TestAnUnverifiedReleaseIsNeverApplied(t *testing.T) {
	r := newApplyRig(t)
	r.stub.rep = reportUnknown
	if code := r.apply(nil); code != 1 || len(r.inst.runs) != 0 || !strings.Contains(r.out.String(), "could not be verified") {
		t.Fatalf("code=%d runs=%d\n%s", code, len(r.inst.runs), r.out.String())
	}
}

func TestInstallerPlanFailuresBlockWithTheReason(t *testing.T) {
	r := newApplyRig(t)
	r.h.env.runInstaller = func(_ context.Context, s installerSpec) installerOutcome {
		return installerOutcome{ExitCode: 1, Stderr: "[baft-install] cannot read BAFT_AGENT_TOKEN_FILE\n"}
	}
	if code := r.apply(nil); code != 1 || !strings.Contains(r.out.String(), "the installer could not plan this update: exit 1: [baft-install] cannot read BAFT_AGENT_TOKEN_FILE") {
		t.Fatalf("code=%d\n%s", code, r.out.String())
	}
}

func TestRoleMapping(t *testing.T) {
	for role, want := range map[string]string{"dialer": "ir", "listener": "ex"} {
		if got, ok := installerRole(role); !ok || got != want {
			t.Errorf("%s -> %q, %v", role, got, ok)
		}
	}
	if _, ok := installerRole("relay"); ok {
		t.Error("an unknown role was accepted")
	}
	r := newApplyRig(t)
	if tg, problems := buildTarget(r.layout, compTransport, r.h.env.inv()); len(problems) != 0 || strings.Join(tg.Args, " ") != "--role ir" {
		t.Fatalf("transport target = %+v %v", tg, problems)
	}
}

func TestParsePlan(t *testing.T) {
	if p, err := parsePlan(`{"state":"ALREADY_INSTALLED","why":"x","changes":1,"steps":[{"item":"baft binary","action":"update","detail":"d"}],"future":true}`); err != nil || len(p.Steps) != 1 {
		t.Fatalf("%+v %v", p, err)
	}
	for _, bad := range []string{"", "not json", `{"why":"no state"}`, `[]`} {
		if _, err := parsePlan(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestOnlyOneUpdateRunsAtATime(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "update-logs")
	lock, err := lockUpdates(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lockUpdates(dir); err == nil || !strings.Contains(err.Error(), "another baft update is running") {
		t.Fatalf("second lock: %v", err)
	}
	lock.Close()
	l2, err := lockUpdates(dir)
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	l2.Close()
}

// ---- the real process runner ----

func bashScript(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "installer.sh")
	if err := os.WriteFile(p, []byte("#!/usr/bin/env bash\n"+body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestHostRunnerPlanRunUsesExactlyTheGivenEnvironment(t *testing.T) {
	t.Setenv("LEAK", "yes")
	script := bashScript(t, `echo "{\"v\":\"$BAFT_X\",\"leak\":\"${LEAK:-none}\",\"args\":\"$*\"}"; echo err >&2; exit 3`)
	o := hostRunInstaller(context.Background(), installerSpec{Script: script, Args: []string{"--role", "ir", "--plan"}, Env: []string{"BAFT_X=1", "PATH=" + os.Getenv("PATH")}})
	if o.Err != nil || o.ExitCode != 3 || !strings.Contains(o.Stdout, `"v":"1"`) || !strings.Contains(o.Stdout, `"leak":"none"`) || !strings.Contains(o.Stdout, "--role ir --plan") || !strings.Contains(o.Stderr, "err") {
		t.Fatalf("outcome = %+v", o)
	}
}

func TestHostRunnerApplyRunsDetachedAndSurvivesAnInterrupt(t *testing.T) {
	sidFile := filepath.Join(t.TempDir(), "sid")
	script := bashScript(t, `echo start; echo $$ > `+sidFile+`; sleep 1; echo finished; exit 0`)
	logPath := filepath.Join(t.TempDir(), "logs", "run.log")
	var tee bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(300 * time.Millisecond); cancel() }()
	o := hostRunInstaller(ctx, installerSpec{Script: script, Args: []string{"--yes"}, Env: []string{"PATH=" + os.Getenv("PATH")}, Detach: true, LogPath: logPath, Tee: &tee})
	if o.Err != nil || o.ExitCode != 0 {
		t.Fatalf("outcome = %+v", o)
	}
	for _, w := range []string{"start", "finished", "interrupt received: the installer keeps running to completion"} {
		if !strings.Contains(tee.String(), w) {
			t.Errorf("terminal output lacks %q:\n%s", w, tee.String())
		}
	}
	if b, err := os.ReadFile(logPath); err != nil || !strings.Contains(string(b), "finished") {
		t.Fatalf("log = %q, %v", b, err)
	}
	if st, _ := os.Stat(logPath); st.Mode().Perm() != 0o600 {
		t.Errorf("log mode = %v", st.Mode().Perm())
	}
	if _, err := os.Stat(sidFile); err != nil {
		t.Fatal("the installer did not run")
	}
}

func TestHostRunnerApplyPropagatesFailureAndRefusesAnExistingLog(t *testing.T) {
	script := bashScript(t, `echo broken >&2; exit 7`)
	logPath := filepath.Join(t.TempDir(), "run.log")
	var tee bytes.Buffer
	o := hostRunInstaller(context.Background(), installerSpec{Script: script, Args: []string{"--yes"}, Env: []string{"PATH=" + os.Getenv("PATH")}, Detach: true, LogPath: logPath, Tee: &tee})
	if o.Err != nil || o.ExitCode != 7 || !strings.Contains(tee.String(), "broken") {
		t.Fatalf("outcome = %+v tee=%q", o, tee.String())
	}
	o2 := hostRunInstaller(context.Background(), installerSpec{Script: script, Args: []string{"--yes"}, Env: nil, Detach: true, LogPath: logPath, Tee: &tee})
	if o2.Err == nil {
		t.Fatal("an existing log was reused")
	}
}

func TestHostRunnerApplyIsInItsOwnSession(t *testing.T) {
	script := bashScript(t, `ps -o sid= -p $$; ps -o pgid= -p $$; echo $$`)
	var tee bytes.Buffer
	o := hostRunInstaller(context.Background(), installerSpec{Script: script, Args: []string{"--yes"}, Env: []string{"PATH=" + os.Getenv("PATH")}, Detach: true, LogPath: filepath.Join(t.TempDir(), "l.log"), Tee: &tee})
	if o.ExitCode != 0 || o.Err != nil {
		t.Fatalf("outcome = %+v tee=%q", o, tee.String())
	}
	f := strings.Fields(tee.String())
	if len(f) != 3 {
		t.Skipf("ps is not available here: %q", tee.String())
	}
	if f[0] != f[2] {
		t.Errorf("session %s != pid %s: the installer does not lead its own session", f[0], f[2])
	}
	if mine, err := exec.Command("ps", "-o", "sid=", "-p", itoa(os.Getpid())).Output(); err == nil && f[0] == strings.TrimSpace(string(mine)) {
		t.Error("the installer shares this process's session")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// signalWriter collects what is written and closes seen on the first write.
type signalWriter struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	seen chan struct{}
	once sync.Once
}

func (w *signalWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	n, err := w.buf.Write(p)
	w.mu.Unlock()
	w.once.Do(func() { close(w.seen) })
	return n, err
}

func (w *signalWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func TestTheUpdateLockOutlivesTheProcessThatTookIt(t *testing.T) {
	// The installer is handed the lock. If the CLI dies (a dropped SSH session)
	// while it runs, a second update must still be refused until it ends.
	dir := filepath.Join(t.TempDir(), "update-logs")
	lock, err := lockUpdates(dir)
	if err != nil {
		t.Fatal(err)
	}
	release := filepath.Join(t.TempDir(), "release")
	// Bounded, and released on cleanup, so a regression fails fast and leaves nothing running.
	t.Cleanup(func() { os.WriteFile(release, nil, 0o600) })
	script := bashScript(t, `[ -e /proc/$$/fd/3 ] && echo fd3-open; for i in $(seq 200); do [ -e `+release+` ] && break; sleep 0.05; done; echo done`)
	tee := &signalWriter{seen: make(chan struct{})}
	logPath := filepath.Join(t.TempDir(), "l.log")
	done := make(chan installerOutcome, 1)
	go func() {
		done <- hostRunInstaller(context.Background(), installerSpec{Script: script, Args: []string{"--yes"}, Env: []string{"PATH=" + os.Getenv("PATH")},
			Detach: true, LogPath: logPath, Tee: tee, Lock: lock})
	}()
	<-tee.seen   // the installer is running: its first output reached the terminal
	lock.Close() // the CLI's own reference goes away, as if it had died
	if _, err := lockUpdates(dir); err == nil {
		t.Fatal("the lock was released while the installer was still running")
	}
	if err := os.WriteFile(release, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if o := <-done; o.ExitCode != 0 || o.Err != nil || !strings.Contains(tee.String(), "fd3-open") {
		t.Fatalf("outcome = %+v tee=%q", o, tee.String())
	}
	var l3 *os.File
	for i := 0; i < 40 && l3 == nil; i++ {
		if l3, err = lockUpdates(dir); err != nil {
			time.Sleep(50 * time.Millisecond)
		}
	}
	if l3 == nil {
		t.Fatalf("the lock was not released after the installer ended: %v", err)
	}
	l3.Close()
}
