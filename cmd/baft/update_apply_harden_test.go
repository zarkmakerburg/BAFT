package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A root process must not run whatever the invoking user's PATH or HOME point
// at: not the interpreter, not python's per-user site directory, not a
// TMPDIR the caller can write to.
func TestInstallerEnvIgnoresTheCallersPathHomeAndTmpdir(t *testing.T) {
	t.Setenv("PATH", "/home/evil/bin:/usr/bin")
	t.Setenv("HOME", "/home/evil")
	t.Setenv("TMPDIR", "relative-tmp")
	t.Setenv("PYTHONPATH", "/home/evil/py")
	t.Setenv("BASH_ENV", "/home/evil/rc")
	t.Setenv("LD_PRELOAD", "/home/evil/x.so")
	env := installerEnv(menuCfg{releaseState: "/opt/baft/release-state.json", unitDir: "/etc/systemd/system"},
		updateTarget{}, "v0.3.4", "k", "/opt/baft/update-tmp/run-1")
	m := map[string]string{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		m[k] = v
	}
	for k, want := range map[string]string{
		"PATH": trustedPath, "HOME": "/root", "PYTHONNOUSERSITE": "1", "TMPDIR": "/opt/baft/update-tmp/run-1",
	} {
		if m[k] != want {
			t.Errorf("%s = %q, want %q", k, m[k], want)
		}
	}
	for _, k := range []string{"PYTHONPATH", "BASH_ENV", "LD_PRELOAD", "ENV"} {
		if _, ok := m[k]; ok {
			t.Errorf("%s reached the installer", k)
		}
	}
}

func TestTheInterpreterIsAnAbsoluteSystemPath(t *testing.T) {
	if bashPath == "" || !filepath.IsAbs(bashPath) {
		t.Fatalf("bashPath = %q", bashPath)
	}
	// A bash earlier on the caller's PATH must not be what runs, in a plan run
	// or in a detached apply run.
	for _, detach := range []bool{false, true} {
		dir := t.TempDir()
		marker := filepath.Join(dir, "ran")
		fake := filepath.Join(dir, "bash")
		if err := os.WriteFile(fake, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
		spec := installerSpec{Script: bashScript(t, "exit 0"), Env: []string{"PATH=" + trustedPath}}
		if detach {
			spec.Detach, spec.LogPath = true, filepath.Join(dir, "run.log")
		}
		o := hostRunInstaller(context.Background(), spec)
		if o.Err != nil || o.ExitCode != 0 {
			t.Fatalf("detach=%v: %+v", detach, o)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Fatalf("detach=%v: the bash from the caller's PATH ran", detach)
		}
	}
}

func TestScratchDirIsAbsoluteAndIgnoresARelativeTmpdir(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("TMPDIR", "tmp")
	if err := os.Mkdir("tmp", 0o700); err != nil {
		t.Fatal(err)
	}
	d, cleanup, err := scratchDir(filepath.Join(t.TempDir(), "release-state.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if !filepath.IsAbs(d) {
		t.Fatalf("scratch dir %q is relative; the hash check and the run would resolve it differently", d)
	}
}

func TestScratchDirAsRootLivesUnderTheRootOnlyPrefix(t *testing.T) {
	prefix := filepath.Join(t.TempDir(), "baft")
	state := filepath.Join(prefix, "release-state.json")
	d, cleanup, err := scratchDir(state, true)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(filepath.Dir(d)) != prefix || !strings.HasPrefix(filepath.Base(d), "run-") {
		t.Fatalf("scratch dir = %s", d)
	}
	if info, _ := os.Stat(d); info == nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("scratch dir mode: %v", info)
	}
	cleanup()
	if _, err := os.Stat(filepath.Join(prefix, "update-tmp")); err == nil {
		t.Error("the holding directory was left behind: a preview must leave nothing on the host")
	}
	if os.Geteuid() != 0 {
		t.Skip("ownership is only enforced when really running as root")
	}
	if err := os.Chmod(prefix, 0o777); err != nil {
		t.Fatal(err)
	}
	if _, _, err := scratchDir(state, true); err == nil {
		t.Fatal("a prefix writable by others was accepted as the home of a root-run installer")
	}
}

func TestStaleScratchDirectoriesArePruned(t *testing.T) {
	parent := t.TempDir()
	old := filepath.Join(parent, "run-old")
	fresh := filepath.Join(parent, "run-fresh")
	other := filepath.Join(parent, "keep-me")
	for _, d := range []string{old, fresh, other} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-48 * time.Hour)
	for _, d := range []string{old, other} {
		os.Chtimes(d, past, past)
	}
	pruneScratch(parent, 24*time.Hour)
	if _, err := os.Stat(old); err == nil {
		t.Error("a day-old run directory was kept")
	}
	for _, d := range []string{fresh, other} {
		if _, err := os.Stat(d); err != nil {
			t.Errorf("%s was removed", d)
		}
	}
}

// What the installer prints can contain text from files fetched over the
// network. The terminal gets it with control characters removed; the log on
// disk keeps every byte.
func TestTheTerminalMirrorOfTheInstallerLogIsSanitised(t *testing.T) {
	evil := "revocation list: signed by key \x1b]52;c;ZXZpbA==\x07\x1b[2J\x1b[Hspoofed\r\nnext line\u009b31m\n"
	script := bashScript(t, "printf '%s' "+shellQuote(evil)+" >&2")
	logPath := filepath.Join(t.TempDir(), "x.log")
	var tee bytes.Buffer
	o := hostRunInstaller(context.Background(), installerSpec{Script: script, Env: []string{"PATH=" + trustedPath}, Detach: true, LogPath: logPath, Tee: &tee})
	if o.Err != nil || o.ExitCode != 0 {
		t.Fatalf("%+v", o)
	}
	got := tee.String()
	for _, c := range []string{"\x1b", "\x07", "\r", "\u009b"} {
		if strings.Contains(got, c) {
			t.Errorf("mirror contains control character %q: %q", c, got)
		}
	}
	if !strings.Contains(got, "spoofed") || !strings.Contains(got, "next line31m\n") {
		t.Errorf("the readable text was lost: %q", got)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil || !strings.Contains(string(raw), "\x1b]52;") {
		t.Errorf("the log must keep what the installer wrote: %q (%v)", raw, err)
	}
}

func TestLineSanitizerKeepsLinesAndFlushesTheLastOne(t *testing.T) {
	var out bytes.Buffer
	l := &lineSanitizer{w: &out}
	l.Write([]byte("one\ttab\ntw"))
	l.Write([]byte("o\x1b[31m"))
	l.Flush()
	if got, want := out.String(), "one\ttab\ntwo[31m\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInstallerFailureShowsTheLastThreeLinesEachOnItsOwn(t *testing.T) {
	o := installerOutcome{ExitCode: 7, Stderr: "l1\nl2\n\x1b[31ml3\x1b[0m\nl4\nl5\n"}
	got := installerFailure(o)
	if got != "exit 7: l3[31m | l4 | l5" && got != "exit 7: l3[0m | l4 | l5" {
		// control bytes are gone; the lines stay separate
		if strings.Contains(got, "l1") || strings.Contains(got, "l2") || strings.Count(got, "|") != 2 || strings.Contains(got, "\x1b") {
			t.Fatalf("got %q", got)
		}
	}
	if strings.Contains(installerFailure(installerOutcome{ExitCode: 1, Stderr: "a\nb\n"}), "ab") {
		t.Error("lines were glued together")
	}
}

// ---- the release root / source in the environment ----

func TestYesRefusesAReleaseSourceTakenFromTheEnvironment(t *testing.T) {
	t.Setenv("BAFT_RELEASE_URL", "https://lab.example/dl")
	r := newApplyRig(t)
	if code := r.apply(nil); code != 1 || len(r.inst.modes("--yes")) != 0 {
		t.Fatalf("code=%d applied=%v\n%s%s", code, r.inst.modes("--yes"), r.out.String(), r.errOut.String())
	}
	if !strings.Contains(r.errOut.String(), "--allow-root-override") || !strings.Contains(r.out.String(), "BAFT_RELEASE_URL") {
		t.Errorf("the refusal does not name the variable and the flag:\n%s%s", r.out.String(), r.errOut.String())
	}

	r2 := newApplyRig(t)
	if code := r2.apply(func(p *applyParams) { p.allowOverride = true }); code != 0 {
		t.Fatalf("explicitly allowed: code=%d\n%s%s", code, r2.out.String(), r2.errOut.String())
	}
}

func TestTheTypedConfirmationNamesTheOverride(t *testing.T) {
	t.Setenv("BAFT_RELEASE_URL", "https://lab.example/dl")
	r := newApplyRig(t)
	var asked string
	r.confirm = func(q string) bool { asked = q; return true }
	if code := r.apply(func(p *applyParams) { p.yes = false }); code != 0 {
		t.Fatalf("code=%d\n%s%s", code, r.out.String(), r.errOut.String())
	}
	if !strings.Contains(asked, "BAFT_RELEASE_URL") || !strings.Contains(asked, "not the production defaults") {
		t.Errorf("the question a person answers hides the override: %q", asked)
	}
}

func TestNoOverrideNoFuss(t *testing.T) {
	for _, k := range []string{"BAFT_ROOT_PUB", "BAFT_RELEASE_URL", "BAFT_GITHUB_REPO", "BAFT_REVOCATIONS_URL"} {
		t.Setenv(k, "")
	}
	if got := overrideEnv(); len(got) != 0 {
		t.Fatalf("overrideEnv = %v", got)
	}
	r := newApplyRig(t)
	if code := r.apply(nil); code != 0 {
		t.Fatalf("code=%d\n%s", code, r.out.String())
	}
}

// ---- what a service is really running ----

func TestProcDigestSeesTheBinaryAProcessWasStartedFromNotTheFileNow(t *testing.T) {
	dir := t.TempDir()
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("no sleep")
	}
	src, err := os.ReadFile(sleep)
	if err != nil {
		t.Skip(err)
	}
	exe := filepath.Join(dir, "svc")
	if err := os.WriteFile(exe, src, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "30")
	if err := cmd.Start(); err != nil {
		t.Skip("cannot start a copy of sleep: ", err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	want, _ := sha256File(exe)

	// Replace the file the way install(1) does: a new file in its place.
	next := filepath.Join(dir, "svc.new")
	if err := os.WriteFile(next, append([]byte{}, append(src, 0)...), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(next, exe); err != nil {
		t.Fatal(err)
	}
	onDisk, _ := sha256File(exe)
	if onDisk == want {
		t.Fatal("test setup: the file did not change")
	}
	got, err := procDigest(cmd.Process.Pid)
	if err != nil {
		t.Skip("cannot read /proc/PID/exe here: ", err)
	}
	if got != want {
		t.Errorf("procDigest = %s, want the digest of the running (old) binary %s, not the file on disk %s", got, want, onDisk)
	}
}

// ---- the plan the installer makes after it has downloaded the release ----

func (r *applyRig) bccUnitPath() string {
	return filepath.Join(r.layout.unitDir, r.layout.bccService+".service")
}

func TestAStepOnlyTheDownloadedPlanHasIsStillRefused(t *testing.T) {
	// install.sh adds "packages" (an apt run as root) only after it has
	// downloaded; --plan never shows it, a real run with --yes would just do it.
	for name, step := range map[string]planStep{
		"apt":          {"packages", "create", "install the missing tools with apt"},
		"go toolchain": {"go toolchain", "create", "install Go into /usr/local/go"},
	} {
		t.Run(name, func(t *testing.T) {
			r := newApplyRig(t)
			p := defaultPlan(compTransport, true)
			p.Steps = append(p.Steps, step)
			p.Changes++
			r.inst.staged["ir"] = p
			if code := r.apply(nil); code != 1 || len(r.inst.modes("--yes")) != 0 {
				t.Fatalf("code=%d applied=%v\n%s", code, r.inst.modes("--yes"), r.out.String())
			}
			if o := r.out.String(); !strings.Contains(o, "does not know") || !strings.Contains(o, step.Item) {
				t.Errorf("the refusal does not name the step:\n%s", o)
			}
		})
	}
}

func TestTheBCCUnitCommentThatMovesWithTheBootstrapScriptIsAllowedAndChecked(t *testing.T) {
	staged := defaultPlan(compBCC, true)
	for i, s := range staged.Steps {
		switch s.Item {
		case "BCC bootstrap script":
			staged.Steps[i].Action = "update"
		case "BCC systemd unit":
			staged.Steps[i].Action = "update"
		}
	}
	staged.Changes += 2

	// The installer rewrites the unit; only the bootstrap hash line moves.
	r := newApplyRig(t)
	r.inst.staged["--bcc-only"] = staged
	r.inst.onApply = func(mode string) {
		if mode != "--bcc-only" {
			return
		}
		b, _ := os.ReadFile(r.bccUnitPath())
		os.WriteFile(r.bccUnitPath(), []byte(strings.Replace(string(b), "# baft-bootstrap-sha256: absent", "# baft-bootstrap-sha256: "+hexSum("new installer"), 1)), 0o644)
	}
	if code := r.apply(nil); code != 0 {
		t.Fatalf("code=%d\n%s%s", code, r.out.String(), r.errOut.String())
	}
	if o := r.out.String(); !strings.Contains(o, "only that line changes") || strings.Contains(o, "WARNING") {
		t.Errorf("output:\n%s", o)
	}

	// The same plan, but the installer changes the listen address as well.
	r2 := newApplyRig(t)
	r2.inst.staged["--bcc-only"] = staged
	r2.inst.onApply = func(mode string) {
		if mode != "--bcc-only" {
			return
		}
		b, _ := os.ReadFile(r2.bccUnitPath())
		s := strings.Replace(string(b), "# baft-bootstrap-sha256: absent", "# baft-bootstrap-sha256: x", 1)
		os.WriteFile(r2.bccUnitPath(), []byte(strings.Replace(s, "127.0.0.1:8080", "0.0.0.0:8080", 1)), 0o644)
	}
	if code := r2.apply(nil); code != 1 {
		t.Fatalf("a unit changed beyond the hash comment, code=%d\n%s", code, r2.out.String())
	}
	if o := r2.out.String(); !strings.Contains(o, "WARNING: the systemd unit of bcc (") || strings.Contains(o, "Updated to v0.3.4") {
		t.Errorf("output:\n%s", o)
	}
}

func TestABCCUnitRewriteWithoutTheBootstrapUpdateThatExplainsItIsRefused(t *testing.T) {
	r := newApplyRig(t)
	p := defaultPlan(compBCC, true)
	for i, s := range p.Steps {
		if s.Item == "BCC systemd unit" {
			p.Steps[i].Action = "update"
		}
	}
	p.Changes++
	r.inst.staged["--bcc-only"] = p
	if code := r.apply(nil); code != 1 || len(r.inst.modes("--yes")) != 0 {
		t.Fatalf("code=%d applied=%v\n%s", code, r.inst.modes("--yes"), r.out.String())
	}
	if o := r.out.String(); !strings.Contains(o, `would update "BCC systemd unit", which an update never does`) {
		t.Errorf("output:\n%s", o)
	}
}

func TestAnyOtherUnitChangeAfterAnUpdateIsReported(t *testing.T) {
	r := newApplyRig(t)
	r.inst.onApply = func(mode string) {
		if mode == "ir" {
			p := filepath.Join(r.layout.unitDir, r.layout.service+".service")
			b, _ := os.ReadFile(p)
			os.WriteFile(p, append(b, []byte("# surprise\n")...), 0o644)
		}
	}
	if code := r.apply(nil); code != 1 {
		t.Fatalf("code=%d\n%s", code, r.out.String())
	}
	if o := r.out.String(); !strings.Contains(o, "WARNING: the systemd unit of transport (") {
		t.Errorf("output:\n%s", o)
	}
}

func TestTheStagedPlanIsWhatThePersonConfirms(t *testing.T) {
	r := newApplyRig(t)
	p := defaultPlan(compBCC, true)
	p.Steps = append([]planStep{{"BCC binary", "update", "replace with the verified release v0.3.4"}}, p.Steps[1:]...)
	r.inst.staged["--bcc-only"] = p
	var shown string
	r.confirm = func(string) bool { shown = r.out.String(); return true }
	if code := r.apply(func(p *applyParams) { p.yes = false }); code != 0 {
		t.Fatalf("code=%d\n%s", code, r.out.String())
	}
	if !strings.Contains(shown, "replace with the verified release v0.3.4") || strings.Contains(shown, "decided by version before the download") {
		t.Errorf("the preview is not the plan after the download:\n%s", shown)
	}
}

func TestAPlanAfterTheDownloadThatCannotBeReadOrRefusesBlocks(t *testing.T) {
	for name, tc := range map[string]struct {
		out   installerOutcome
		wants string
	}{
		"refused":  {installerOutcome{ExitCode: 3, Stderr: "refusing to touch a broken install: x"}, "could not stage this update: exit 3"},
		"no plan":  {installerOutcome{ExitCode: exitInstallerAsks, Stdout: "not json\n"}, "could not stage this update"},
		"not run":  {installerOutcome{Err: errors.New("fork failed")}, "could not stage this update: fork failed"},
		"it acted": {installerOutcome{Stdout: `{"state":"ALREADY_INSTALLED","changes":2,"steps":[]}`}, "applied 2 change(s) without asking"},
	} {
		t.Run(name, func(t *testing.T) {
			r := newApplyRig(t)
			base := r.inst.run
			r.h.env.runInstaller = func(ctx context.Context, s installerSpec) installerOutcome {
				if s.Args[len(s.Args)-1] == "--json" && !contains(s.Args, "--plan") && modeOf(s.Args) == "ir" {
					r.inst.runs = append(r.inst.runs, s)
					return tc.out
				}
				return base(ctx, s)
			}
			if code := r.apply(nil); code != 1 || contains(r.inst.modes("--yes"), "ir") {
				t.Fatalf("code=%d applied=%v\n%s", code, r.inst.modes("--yes"), r.out.String())
			}
			if o := r.out.String(); !strings.Contains(o, tc.wants) {
				t.Errorf("output lacks %q:\n%s", tc.wants, o)
			}
		})
	}
}

func TestAComponentWithNothingLeftToChangeIsNotRunAgain(t *testing.T) {
	r := newApplyRig(t)
	done := defaultPlan(compTransport, false)
	r.inst.staged["ir"] = done
	r.inst.applied["ir"] = true
	if code := r.apply(nil); code != 0 {
		t.Fatalf("code=%d\n%s", code, r.out.String())
	}
	if got := strings.Join(r.inst.modes("--yes"), ","); got != "--bcc-only,--agent-only" {
		t.Fatalf("applied %s", got)
	}
	if o := r.out.String(); !strings.Contains(o, "transport: the installer has nothing left to change here") {
		t.Errorf("output:\n%s", o)
	}
}

func TestTheStagedPlanNeedsRootAndAPreviewAsRootStillChangesNothing(t *testing.T) {
	r := newApplyRig(t)
	r.h.env.geteuid = func() int { return 1000 }
	if code := r.apply(func(p *applyParams) { p.planOnly = true }); code != 0 {
		t.Fatalf("code=%d\n%s", code, r.out.String())
	}
	if len(r.inst.stagedModes()) != 0 || !strings.Contains(r.out.String(), "checked when this runs as root") {
		t.Errorf("staged=%v\n%s", r.inst.stagedModes(), r.out.String())
	}
	r2 := newApplyRig(t)
	if code := r2.apply(func(p *applyParams) { p.planOnly = true }); code != 0 {
		t.Fatalf("code=%d\n%s", code, r2.out.String())
	}
	if len(r2.inst.stagedModes()) != 3 || len(r2.inst.modes("--yes")) != 0 {
		t.Errorf("staged=%v applied=%v", r2.inst.stagedModes(), r2.inst.modes("--yes"))
	}
}

func TestARunningServiceThatIsNotEnabledIsExplainedNotCalledUnknown(t *testing.T) {
	// install.sh counts a running agent whose unit is disabled as healthy and
	// plans "agent service enable"; refusing it as "unknown" would hide the cause.
	r := newApplyRig(t)
	p := defaultPlan(compAgent, true)
	p.Steps = append(p.Steps, planStep{"agent service enable", "update", "enable baft-agent.service"})
	p.Changes++
	r.inst.plans = map[string]installerPlan{"--agent-only": p}
	if code := r.apply(nil); code != 1 || len(r.inst.modes("--yes")) != 0 {
		t.Fatalf("code=%d applied=%v\n%s", code, r.inst.modes("--yes"), r.out.String())
	}
	o := r.out.String()
	if !strings.Contains(o, "agent service is running but not enabled at boot") || strings.Contains(o, "does not know") {
		t.Errorf("output:\n%s", o)
	}
}

func TestAPreviewAsRootLeavesNothingUnderTheInstallPrefix(t *testing.T) {
	r := newApplyRig(t)
	prefix := filepath.Dir(r.layout.releaseState)
	before, _ := os.ReadDir(prefix)
	if code := r.apply(func(p *applyParams) { p.planOnly = true }); code != 0 {
		t.Fatalf("code=%d\n%s", code, r.out.String())
	}
	after, _ := os.ReadDir(prefix)
	if len(after) != len(before) {
		var names []string
		for _, e := range after {
			names = append(names, e.Name())
		}
		t.Errorf("a preview left %v under %s", names, prefix)
	}
}
