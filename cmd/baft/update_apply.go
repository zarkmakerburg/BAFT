package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/zarkmakerburg/baft/internal/config"
	"github.com/zarkmakerburg/baft/internal/release"
	"github.com/zarkmakerburg/baft/internal/selfupdate"
	"github.com/zarkmakerburg/baft/internal/uninstall"
)

// `baft update apply`: bring the installed components to the newest signed
// release by running the release's own signed installer (install.sh), once per
// component this host has, in the mode that component was installed in.
//
//	baft.service        -> --role ex|ir   (role read from the node's config)
//	baft-agent.service  -> --agent-only   (inputs recovered from its unit)
//	baft-bcc.service    -> --bcc-only
//
// This code decides nothing about files, keys or units itself. The installer
// does the work, with its own backup, verification and rollback. What this
// adds is the gating around it:
//
//   - the installer is the verified, signed one (sha256 checked again right
//     before every run) and the version is pinned to the one that verified;
//   - every installer plan is read first (`--plan --json`) and refused unless
//     the install is healthy and every step is on a short allow-list: binaries,
//     the release state and restarts may change; keys, certificates, config,
//     pairing, tokens, access files and units may not. A step this baft does not
//     know is refused as well, never waved through;
//   - nothing runs before one explicit confirmation of the whole plan;
//   - the installer runs in its own session with its output in a log file, so
//     a dropped SSH session or Ctrl-C cannot leave a half-applied install;
//   - BCC is only updated when a recent backup of it exists.

const (
	planTimeout     = 90 * time.Second
	stagedTimeout   = 10 * time.Minute
	backupFreshness = 26 * time.Hour

	exitNeedsConfirmation = 4
	exitInstallerAsks     = 4 // install.sh: the plan changes an existing install and nothing confirmed it

	installerHealthy = "ALREADY_INSTALLED"
)

// healthyStates are the installer states of an install that is present, paired
// and running. UPGRADE_AVAILABLE and CURRENT are refinements of ALREADY_INSTALLED
// (the installer only reaches them from it); every other state (FRESH_INSTALL,
// PARTIAL_INSTALL, REPAIR_REQUIRED, BROKEN_INSTALL) is not something to update.
var healthyStates = map[string]bool{installerHealthy: true, "UPGRADE_AVAILABLE": true, "CURRENT": true}

// applyOrder: BCC first (it shares nothing with the other two), then the
// transport service, then the agent. baft and baft-pair are files the
// transport service runs and the agent installer also installs, and an
// installer restarts only its own service: whichever of the two replaces them
// first must be the transport, or the tunnel would keep running the old
// process while its files already look current.
var applyOrder = []string{compBCC, compTransport, compAgent}

type installerSpec struct {
	Script  string
	Args    []string
	Env     []string
	Detach  bool      // apply: own session, output to LogPath, survives this process
	LogPath string    // Detach only
	Tee     io.Writer // Detach only: mirrors the log to the terminal
	// Lock, for a Detach run, is handed to the installer (as fd 3) so the update
	// lock lasts as long as the installer does, even if this process dies.
	Lock *os.File
}

type installerOutcome struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error // the installer could not be started
}

type planStep struct {
	Item   string `json:"item"`
	Action string `json:"action"`
	Detail string `json:"detail"`
}

type installerPlan struct {
	State   string     `json:"state"`
	Why     string     `json:"why"`
	Changes int        `json:"changes"`
	Steps   []planStep `json:"steps"`
}

type updateTarget struct {
	Component string
	Unit      string
	Args      []string
	Env       map[string]string
	BackupDir string // bcc only
	plan      installerPlan
	staged    bool // plan is the one printed after the release was downloaded and verified
	nothing   bool // that plan has nothing to change: the component is not run again
}

// allowedSteps is every installer plan step an update may contain, with the
// actions it may have. Anything else stops the update.
var allowedSteps = func() map[string]map[string]bool {
	m := map[string]map[string]bool{}
	set := func(actions string, items ...string) {
		for _, it := range items {
			m[it] = map[string]bool{}
			for _, a := range strings.Fields(actions) {
				m[it][a] = true
			}
		}
	}
	set("keep update", "baft binary", "baft-pair binary", "baft-agent binary", "BCC binary", "release state", "BCC bootstrap script")
	set("keep restart", "service", "agent service", "BCC service")
	// Credentials, identity, pairing and units: never touched by an update.
	// (A unit rewrite could silently change a panel's listen address.)
	set("keep", "service user", "directories", "noise key", "outer PKI", "config", "systemd unit", "credentials",
		"agent token", "release root pin", "agent unit",
		"BCC directories", "BCC admin token", "BCC access", "BCC job key", "BCC systemd unit")
	set("skip", "pairing")
	set("keep skip", "BCC SSH enrollment")
	return m
}()

// vetPlan returns why a plan may not be applied; none means it may.
func vetPlan(p installerPlan) []string { return vetPlanWith(p, nil) }

// vetPlanWith is vetPlan with one narrow exception hook: allow may accept a
// step the allow-list refuses.
func vetPlanWith(p installerPlan, allow func(planStep) bool) []string {
	var out []string
	if !healthyStates[p.State] {
		out = append(out, fmt.Sprintf("the install is %s (%s), not a healthy install to update; repair it first", clean(p.State), clean(p.Why)))
	}
	for _, s := range p.Steps {
		actions, known := allowedSteps[s.Item]
		switch {
		case !known && strings.HasSuffix(s.Item, "service enable"):
			// install.sh calls a running agent healthy even when its unit is not
			// enabled, then plans to enable it; an update never changes that.
			out = append(out, fmt.Sprintf("%s is running but not enabled at boot, and an update does not change that; enable it yourself (systemctl enable) or repair it first", clean(strings.TrimSuffix(s.Item, " enable"))))
		case !known:
			out = append(out, fmt.Sprintf("the installer plans a step this baft does not know (%q %s); apply by hand", clean(s.Item), clean(s.Action)))
		case !actions[s.Action] && !(allow != nil && allow(s)):
			out = append(out, fmt.Sprintf("the installer would %s %q, which an update never does", clean(s.Action), clean(s.Item)))
		}
	}
	return out
}

func hasStep(p installerPlan, item, action string) bool {
	for _, s := range p.Steps {
		if s.Item == item && s.Action == action {
			return true
		}
	}
	return false
}

// vetStagedPlan vets the plan the installer prints after it has downloaded and
// verified the release, i.e. the one it will really apply. It differs from the
// plan made before the download in one known way: the BCC unit records the
// sha256 of the bootstrap script as a comment, and a new release ships a new
// bootstrap script, so the unit is rewritten with that one line changed. That
// is accepted only together with the bootstrap script update that explains it,
// and applyUpdate checks afterwards that nothing else in the unit changed.
func vetStagedPlan(component string, p installerPlan) []string {
	return vetPlanWith(p, func(s planStep) bool {
		return component == compBCC && s.Item == "BCC systemd unit" && s.Action == "update" && hasStep(p, "BCC bootstrap script", "update")
	})
}

// parseStagedPlan finds the plan in a real (not --plan) run's standard output:
// the last line that is a plan.
func parseStagedPlan(stdout string) (installerPlan, error) {
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); strings.HasPrefix(line, "{") {
			if p, err := parsePlan(line); err == nil {
				return p, nil
			}
		}
	}
	return installerPlan{}, errors.New("the installer printed no plan")
}

func parsePlan(stdout string) (installerPlan, error) {
	var p installerPlan
	if err := json.Unmarshal([]byte(stdout), &p); err != nil {
		return p, fmt.Errorf("the installer's plan is not readable: %v", err)
	}
	if p.State == "" {
		return p, errors.New("the installer's plan has no state")
	}
	return p, nil
}

// ---- finding what to update ----

func (e opsEnv) euid() int {
	if e.geteuid != nil {
		return e.geteuid()
	}
	return os.Geteuid()
}

func (e opsEnv) installer(ctx context.Context, s installerSpec) installerOutcome {
	if e.runInstaller != nil {
		return e.runInstaller(ctx, s)
	}
	return hostRunInstaller(ctx, s)
}

func (e opsEnv) inv() inventoryHost {
	if e.inventory != nil {
		return *e.inventory
	}
	return hostInventory
}

// buildTarget recovers, from the installed unit, everything the installer needs
// to treat this component as the same install, so a rerun keeps it as it is.
func buildTarget(l menuCfg, name string, inv inventoryHost) (updateTarget, []string) {
	switch name {
	case compTransport:
		return transportTarget(l, inv)
	case compAgent:
		return agentTarget(l, inv)
	case compBCC:
		return bccTarget(l, inv)
	}
	return updateTarget{}, []string{"unknown component " + name}
}

func transportTarget(l menuCfg, inv inventoryHost) (updateTarget, []string) {
	t := updateTarget{Component: compTransport, Unit: l.service}
	raw, err := inv.readFile(filepath.Join(l.unitDir, l.service+".service"))
	if err != nil {
		return t, []string{fmt.Sprintf("%s: cannot read its unit: %v", compTransport, err)}
	}
	p, ok := uninstall.ParseInstallerServiceUnit(string(raw))
	if !ok {
		return t, []string{fmt.Sprintf("%s: the unit is not the one install.sh writes", compTransport)}
	}
	if filepath.Base(p.Config) != "baft.yaml" {
		return t, []string{fmt.Sprintf("%s: the config is %s; the installer only manages a baft.yaml", compTransport, clean(p.Config))}
	}
	cfg, err := config.LoadFile(p.Config)
	if err != nil {
		return t, []string{fmt.Sprintf("%s: the node's config cannot be loaded: %v", compTransport, err)}
	}
	role, ok := installerRole(cfg.Node.Role)
	if !ok {
		return t, []string{fmt.Sprintf("%s: the node role %q is neither dialer (IR) nor listener (EX)", compTransport, clean(cfg.Node.Role))}
	}
	t.Args = []string{"--role", role}
	t.Env = map[string]string{
		"BAFT_SERVICE": l.service, "BAFT_USER": p.User, "BAFT_BIN": p.Bin,
		"BAFT_PAIR_BIN":   filepath.Join(filepath.Dir(p.Bin), "baft-pair"),
		"BAFT_CONFIG_DIR": filepath.Dir(p.Config), "BAFT_STATE_DIR": p.StateDir,
	}
	return t, nil
}

// installerRole maps a node's configured role to the installer's --role.
func installerRole(nodeRole string) (string, bool) {
	switch nodeRole {
	case "dialer":
		return "ir", true
	case "listener":
		return "ex", true
	}
	return "", false
}

func agentTarget(l menuCfg, inv inventoryHost) (updateTarget, []string) {
	t := updateTarget{Component: compAgent, Unit: l.agentUnit}
	raw, err := inv.readFile(filepath.Join(l.unitDir, l.agentUnit+".service"))
	if err != nil {
		return t, []string{fmt.Sprintf("%s: cannot read its unit: %v", compAgent, err)}
	}
	p, ok := uninstall.ParseInstallerAgentUnit(string(raw))
	if !ok {
		return t, []string{fmt.Sprintf("%s: the unit is not the one install.sh writes", compAgent)}
	}
	var problems []string
	if p.ReleaseState != l.releaseState {
		problems = append(problems, fmt.Sprintf("%s: it records its release in %s, not %s", compAgent, clean(p.ReleaseState), clean(l.releaseState)))
	}
	if p.SystemdDir != l.unitDir {
		problems = append(problems, fmt.Sprintf("%s: it was installed for the unit directory %s, not %s", compAgent, clean(p.SystemdDir), clean(l.unitDir)))
	}
	jobKey, err := inv.readFile(filepath.Join(p.AgentDir, "bcc-job.pub"))
	if err != nil {
		problems = append(problems, fmt.Sprintf("%s: cannot read the pinned BCC job key (run as root): %v", compAgent, err))
	}
	if len(problems) > 0 {
		return t, problems
	}
	t.Args = []string{"--agent-only"}
	t.Env = map[string]string{
		"BAFT_BCC_URL": p.BCCURL, "BAFT_NODE_ID": p.NodeID, "BAFT_AGENT_DIR": p.AgentDir,
		"BAFT_AGENT_TOKEN_FILE": filepath.Join(p.AgentDir, "token"), "BAFT_BCC_JOB_KEY": strings.TrimSpace(string(jobKey)),
		"BAFT_AGENT_STATE_DIR": p.AgentStateDir, "BAFT_METRICS_LISTEN": p.MetricsListen, "BAFT_AGENT_INTERVAL": p.Interval,
		"BAFT_USER": p.User, "BAFT_SERVICE": p.Service, "BAFT_CONFIG_DIR": p.ConfigDir, "BAFT_STATE_DIR": p.BaftStateDir,
		"BAFT_PREFIX": p.Prefix, "BAFT_AGENT_BIN": p.AgentBin, "BAFT_AGENT_UNIT": l.agentUnit,
		"BAFT_BIN": filepath.Join(p.BinDir, "baft"), "BAFT_PAIR_BIN": filepath.Join(p.BinDir, "baft-pair"),
	}
	if p.ReleaseBaseURL != "" {
		t.Env["BAFT_AGENT_RELEASE_BASE_URL"] = p.ReleaseBaseURL
	}
	if p.RevocationsURL != "" {
		t.Env["BAFT_AGENT_REVOCATIONS_URL"] = p.RevocationsURL
	}
	if p.AllowHTTP {
		t.Env["BAFT_AGENT_ALLOW_HTTP"] = "1"
	}
	return t, nil
}

func bccTarget(l menuCfg, inv inventoryHost) (updateTarget, []string) {
	t := updateTarget{Component: compBCC, Unit: l.bccService}
	raw, err := inv.readFile(filepath.Join(l.unitDir, l.bccService+".service"))
	if err != nil {
		return t, []string{fmt.Sprintf("%s: cannot read its unit: %v", compBCC, err)}
	}
	u, ok := parseBCCUnit(string(raw))
	if !ok {
		return t, []string{fmt.Sprintf("%s: the unit is not the BCC unit install.sh writes", compBCC)}
	}
	t.Args = []string{"--bcc-only"}
	t.Env = map[string]string{"BAFT_BCC_SERVICE": l.bccService, "BAFT_BCC_BIN": u.Bin}
	// The listen address, TLS files, public URL and bootstrap script are kept by
	// the installer itself from the unit. The file locations are not, so pass
	// the installed ones; the installer then finds this panel, not a new one.
	for env, path := range map[string]string{
		"BAFT_BCC_STATE_FILE": u.StateFile, "BAFT_BCC_ADMIN_TOKEN_FILE": u.AdminTokenFile,
		"BAFT_BCC_ACCESS_FILE": u.AccessFile, "BAFT_BCC_JOB_KEY_FILE": u.JobKeyFile, "BAFT_BCC_BACKUP_DIR": u.BackupDir,
	} {
		if path != "" {
			t.Env[env] = path
		}
	}
	if u.AdminTokenFile != "" {
		t.Env["BAFT_BCC_CONFIG_DIR"] = filepath.Dir(u.AdminTokenFile)
	}
	if u.StateFile != "" {
		t.Env["BAFT_BCC_STATE_DIR"] = filepath.Dir(u.StateFile)
	}
	t.BackupDir = u.BackupDir
	return t, nil
}

// installerEnv is the complete environment for one installer run. It is built
// from scratch: nothing is inherited that could change which program the
// installer runs as root (PATH and HOME are fixed, Python's per-user site
// directory is off, TMPDIR is the private scratch directory), and only what the
// installer needs from this process is passed on (locale, proxies and
// certificate bundles, and the release source the check used). The downgrade
// switch is always off.
func installerEnv(l menuCfg, t updateTarget, version, rootPub, tmp string) []string {
	env := map[string]string{
		"BAFT_NONINTERACTIVE": "1", "BAFT_INSTALL_FROM": "release", "BAFT_VERSION": version,
		"BAFT_ALLOW_DOWNGRADE": "0", "BAFT_ROOT_PUB": rootPub,
		"BAFT_RELEASE_STATE": l.releaseState, "BAFT_PREFIX": filepath.Dir(l.releaseState), "BAFT_SYSTEMD_DIR": l.unitDir,
		"PATH": trustedPath, "HOME": "/root", "PYTHONNOUSERSITE": "1", "PYTHONDONTWRITEBYTECODE": "1",
	}
	if tmp != "" {
		env["TMPDIR"] = tmp
	}
	for _, k := range []string{"LANG", "LC_ALL", "SSL_CERT_FILE", "SSL_CERT_DIR", "CURL_CA_BUNDLE",
		"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy", "NO_PROXY", "no_proxy",
		"BAFT_GITHUB_REPO", "BAFT_RELEASE_URL", "BAFT_REVOCATIONS_URL"} {
		if v := os.Getenv(k); v != "" {
			env[k] = v
		}
	}
	for k, v := range t.Env {
		env[k] = v
	}
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

// trustedPath is the only PATH a root-run installer gets: system directories,
// never the invoking user's (sudo -E, su without -, cron and systemd units can
// all hand a root process a user's PATH). It is a variable only so that a test
// build can put stub tools first (go build -ldflags "-X main.trustedPath=...");
// nothing at run time can change it.
var trustedPath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

// bashPath is where the installer's interpreter is looked up: an absolute path
// in a system directory, not whatever "bash" resolves to on the caller's PATH.
var bashPath = func() string {
	for _, p := range []string{"/usr/bin/bash", "/bin/bash"} {
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o022 == 0 {
			return p
		}
	}
	return ""
}()

// overrideEnv lists the environment variables that redirect where a release
// comes from or which key it must be signed by, as far as they are set. They
// exist for lab releases signed with throwaway keys; on a production host they
// are a reason to stop and look.
func overrideEnv() []string {
	var set []string
	for _, k := range []string{"BAFT_ROOT_PUB", "BAFT_RELEASE_URL", "BAFT_GITHUB_REPO", "BAFT_REVOCATIONS_URL"} {
		if strings.TrimSpace(os.Getenv(k)) != "" {
			set = append(set, k)
		}
	}
	return set
}

// scratchDir returns a fresh private directory for the verified installer and
// the installer's own staging. As root it lives under the install prefix, which
// must be root-owned and not writable by anyone else: a directory under a
// caller-chosen TMPDIR could be swapped between the sha256 check and the run.
func scratchDir(releaseState string, root bool) (string, error) {
	if !root {
		d, err := os.MkdirTemp("", "baft-update-")
		if err != nil {
			return "", err
		}
		return filepath.Abs(d)
	}
	prefix, err := filepath.Abs(filepath.Dir(releaseState))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(prefix, 0o755); err != nil {
		return "", err
	}
	if err := requireRootOnly(prefix); err != nil {
		return "", err
	}
	parent := filepath.Join(prefix, "update-tmp")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", err
	}
	if err := requireRootOnly(parent); err != nil {
		return "", err
	}
	pruneScratch(parent, 24*time.Hour)
	return os.MkdirTemp(parent, "run-")
}

// requireRootOnly checks, when this process really is root, that dir is owned
// by root and not writable by group or others.
func requireRootOnly(dir string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != 0 || info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%s is not owned by root and closed to others; refusing to run an installer from under it", dir)
	}
	return nil
}

// pruneScratch removes leftovers of runs that died before cleaning up.
func pruneScratch(parent string, olderThan time.Duration) {
	ents, err := os.ReadDir(parent)
	if err != nil {
		return
	}
	for _, e := range ents {
		if info, err := e.Info(); err == nil && e.IsDir() && strings.HasPrefix(e.Name(), "run-") && time.Since(info.ModTime()) > olderThan {
			os.RemoveAll(filepath.Join(parent, e.Name()))
		}
	}
}

// ---- recovery readiness ----

func newestBackup(dir string) (time.Time, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return time.Time{}, err
	}
	var newest time.Time
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".baftbak") {
			continue
		}
		if info, err := e.Info(); err == nil && info.Mode().IsRegular() && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	if newest.IsZero() {
		return newest, errors.New("no backup files")
	}
	return newest, nil
}

// backupProblem says why BCC may not be updated yet, or "" when a recent
// backup of it exists. It checks that one exists and is recent, not that it
// decrypts (`baft-bcc verify-backup` does that).
func backupProblem(t updateTarget, now time.Time, h inventoryHost) string {
	if t.BackupDir == "" {
		return "cannot tell where BCC keeps its backups, so a recent one cannot be confirmed"
	}
	newest, err := h.newestBackup(t.BackupDir)
	if err != nil {
		return fmt.Sprintf("no BCC backup found in %s (%v); take one first (BCC > Backups)", clean(t.BackupDir), err)
	}
	if age := now.Sub(newest); age > backupFreshness {
		return fmt.Sprintf("the newest BCC backup in %s is %s old (limit %s); take a fresh one first", clean(t.BackupDir), age.Round(time.Minute), backupFreshness)
	}
	return ""
}

// ---- the command ----

func runUpdateApply(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, env opsEnv) int {
	var version, only string
	var planOnly, yes, skipBackup, allowOverride bool
	var l menuCfg
	f, ok := parseOpsFlags("update apply", args, stderr, func(fs *flag.FlagSet) {
		fs.StringVar(&version, "version", "", "release to apply (default: the newest)")
		fs.StringVar(&only, "only", "", "comma-separated components: transport, agent, bcc (default: every one that is behind)")
		fs.BoolVar(&planOnly, "plan", false, "show what would change, change nothing")
		fs.BoolVar(&yes, "yes", false, "apply without asking")
		fs.BoolVar(&skipBackup, "skip-backup-check", false, "update BCC without a recent backup of it (not recommended)")
		fs.BoolVar(&allowOverride, "allow-root-override", false, "with --yes: accept a release root or source set by BAFT_ROOT_PUB / BAFT_RELEASE_URL / BAFT_GITHUB_REPO / BAFT_REVOCATIONS_URL (lab releases only)")
		fs.StringVar(&l.unitDir, "unit-dir", "/etc/systemd/system", "systemd unit directory")
		fs.StringVar(&l.agentUnit, "agent-unit", "baft-agent", "agent service name")
		fs.StringVar(&l.bccService, "bcc-service", "baft-bcc", "BCC service name")
	})
	if !ok || f.json {
		updateUsage(stderr)
		return 2
	}
	l.file, l.service, l.releaseState = f.file, f.service, f.releaseState
	sel := map[string]bool{}
	for _, n := range strings.Split(only, ",") {
		if n = strings.TrimSpace(n); n == "" {
			continue
		}
		if n != compTransport && n != compAgent && n != compBCC {
			fmt.Fprintf(stderr, "--only: %q is not transport, agent or bcc\n", clean(n))
			return 2
		}
		sel[n] = true
	}
	in := bufio.NewReader(stdin)
	tty := false
	if f, ok := stdin.(*os.File); ok {
		tty = isTerminal(f)
	}
	if !yes && !planOnly && !tty {
		fmt.Fprintln(stderr, "baft update apply asks for a confirmation and there is no terminal; review with --plan, then re-run with --yes")
		return exitNeedsConfirmation
	}
	return applyUpdate(ctx, applyParams{
		layout: l, version: version, only: sel, planOnly: planOnly, yes: yes, skipBackupCheck: skipBackup, allowOverride: allowOverride,
		confirm: func(q string) bool {
			fmt.Fprint(stdout, q)
			line, _ := in.ReadString('\n')
			return strings.EqualFold(strings.TrimSpace(line), "update")
		},
		out: stdout, errw: stderr, env: env,
	})
}

type applyParams struct {
	layout          menuCfg
	version         string
	only            map[string]bool
	planOnly        bool
	yes             bool
	skipBackupCheck bool
	allowOverride   bool // --yes may use a release root / source taken from the environment
	degradeToPlan   bool // the menu: without root, show the plan and stop instead of failing
	confirm         func(question string) bool
	out, errw       io.Writer
	env             opsEnv
}

func applyUpdate(ctx context.Context, p applyParams) int {
	out, l := p.out, p.layout
	if p.env.euid() != 0 && !p.planOnly {
		if !p.degradeToPlan {
			fmt.Fprintln(p.errw, "baft update apply must run as root (sudo); --plan previews without it")
			return 1
		}
		p.planOnly = true
		fmt.Fprintln(out, "Not running as root, so this shows the plan only. To apply: sudo baft update apply")
	}
	dir, err := scratchDir(l.releaseState, p.env.euid() == 0)
	if err != nil {
		fmt.Fprintln(p.errw, "cannot create a scratch directory:", err)
		return 1
	}
	defer os.RemoveAll(dir)

	check := p.env.checkUpdate
	if check == nil {
		check = hostUpdateCheck
	}
	checkCtx, cancelCheck := context.WithTimeout(ctx, updateCheckTimeout)
	rep := check(checkCtx, l.releaseState, p.version, dir)
	cancelCheck()
	inv := p.env.inv()
	comps := inspectComponents(l, runtime.GOARCH, rep.Artifacts, inv)
	foldInventory(&rep, comps)
	printReleaseFacts(out, rep, comps)
	fmt.Fprintln(out)
	if rep.Installer == "" || rep.Artifacts == nil {
		fmt.Fprintf(out, "The release could not be verified, so nothing is applied. %s\n", clean(rep.Reason))
		return 1
	}

	// What is behind, and what stops us from touching it.
	var behind, blockers []string
	for _, c := range comps {
		if len(p.only) > 0 && !p.only[c.Name] {
			continue
		}
		switch c.Status {
		case statOutdated:
			behind = append(behind, c.Name)
		case statUnknown:
			blockers = append(blockers, fmt.Sprintf("%s (%s): %s", c.Name, clean(c.Unit), clean(c.Reason)))
		}
	}
	for n := range p.only {
		found := false
		for _, c := range comps {
			found = found || c.Name == n
		}
		if !found {
			blockers = append(blockers, fmt.Sprintf("--only %s: that component is not installed here", n))
		}
	}
	if len(blockers) > 0 {
		printBlockers(out, blockers)
		return 1
	}
	if len(behind) == 0 {
		fmt.Fprintf(out, "Nothing to apply: every selected component already runs the signed release %s.\n", clean(rep.Latest))
		return 0
	}

	rootPub, _, err := releaseRoot() // an override is shown with the release facts above
	if err != nil {
		fmt.Fprintln(p.errw, "the release root key is unusable:", err)
		return 1
	}
	rootText := release.EncodePublic(rootPub)
	overrides := overrideEnv()
	if len(overrides) > 0 {
		fmt.Fprintf(out, "NOTE: %s set in this environment: the release is taken from, or checked against, what they name, not the production defaults pinned in baft.\n", strings.Join(overrides, ", "))
		if p.yes && !p.planOnly && !p.allowOverride {
			fmt.Fprintln(p.errw, "refusing --yes with a release root or source taken from the environment (sudo -E, a profile export?). Unset them, or pass --allow-root-override if this is a lab host.")
			return 1
		}
	}
	var targets []*updateTarget
	for _, name := range applyOrder {
		if !contains(behind, name) {
			continue
		}
		t, problems := buildTarget(l, name, inv)
		blockers = append(blockers, problems...)
		if len(problems) == 0 {
			targets = append(targets, &t)
		}
	}
	blockers = append(blockers, sharedBinaryProblems(l, comps, targets, inv)...)
	fullEnv := func(t *updateTarget) []string { return installerEnv(l, *t, rep.Latest, rootText, dir) }
	for _, t := range targets {
		pctx, pcancel := context.WithTimeout(ctx, planTimeout)
		o := p.env.installer(pctx, installerSpec{Script: rep.Installer, Args: append(append([]string{}, t.Args...), "--plan", "--json"), Env: fullEnv(t)})
		pcancel()
		if o.Err != nil || o.ExitCode != 0 {
			blockers = append(blockers, fmt.Sprintf("%s: the installer could not plan this update: %s", t.Component, installerFailure(o)))
			continue
		}
		plan, err := parsePlan(o.Stdout)
		if err != nil {
			blockers = append(blockers, fmt.Sprintf("%s: %v", t.Component, err))
			continue
		}
		t.plan = plan
		for _, why := range vetPlan(plan) {
			blockers = append(blockers, fmt.Sprintf("%s: %s", t.Component, why))
		}
		if t.Component == compBCC && !p.skipBackupCheck {
			if why := backupProblem(*t, time.Now(), inv); why != "" {
				blockers = append(blockers, fmt.Sprintf("%s: %s (or --skip-backup-check)", t.Component, why))
			}
		}
	}
	if len(blockers) > 0 {
		printBlockers(out, blockers)
		return 1
	}

	// The plan above is made before anything is downloaded, so it can only judge
	// binaries by version. The installer's own plan after it has downloaded and
	// verified the release is the one it will really apply: ask for exactly that
	// (a real run without --yes and without a terminal stops, with exit 4, right
	// after printing it) and vet it too. This needs root, like the run itself.
	if p.env.euid() == 0 {
		for _, t := range targets {
			sctx, scancel := context.WithTimeout(ctx, stagedTimeout)
			o := p.env.installer(sctx, installerSpec{Script: rep.Installer, Args: append(append([]string{}, t.Args...), "--json"), Env: fullEnv(t)})
			scancel()
			plan, perr := parseStagedPlan(o.Stdout)
			switch {
			case o.Err != nil:
				blockers = append(blockers, fmt.Sprintf("%s: the installer could not stage this update: %s", t.Component, installerFailure(o)))
			case o.ExitCode == exitInstallerAsks && perr == nil:
				t.plan, t.staged = plan, true
				for _, why := range vetStagedPlan(t.Component, plan) {
					blockers = append(blockers, fmt.Sprintf("%s: %s", t.Component, why))
				}
			case o.ExitCode == 0 && perr == nil && plan.Changes == 0:
				// Nothing left to change (an earlier component's pass updated what
				// they share); the installer ran through without changing anything.
				t.plan, t.staged, t.nothing = plan, true, true
			case o.ExitCode == 0 && perr == nil:
				blockers = append(blockers, fmt.Sprintf("%s: the installer applied %d change(s) without asking; check it with baft doctor and its log", t.Component, plan.Changes))
			default:
				blockers = append(blockers, fmt.Sprintf("%s: the installer could not stage this update: %s", t.Component, installerFailure(o)))
			}
		}
		if len(blockers) > 0 {
			printBlockers(out, blockers)
			return 1
		}
	} else {
		fmt.Fprintln(out, "(The plan the installer makes after downloading is checked when this runs as root.)")
	}

	for _, t := range targets {
		printTargetPlan(out, *t, comps, rep.Latest)
	}
	if p.planOnly {
		fmt.Fprintln(out, "\nThis was a preview. Nothing was changed.")
		return 0
	}
	question := fmt.Sprintf("\nThis restarts the services above and moves them to %s. Type 'update' to apply: ", clean(rep.Latest))
	if len(overrides) > 0 {
		question = fmt.Sprintf("\nThis restarts the services above and moves them to %s, taken from the environment override (%s), not the production defaults. Type 'update' to apply: ", clean(rep.Latest), strings.Join(overrides, ", "))
	}
	if !p.yes && !p.confirm(question) {
		fmt.Fprintln(out, "Not confirmed. Nothing was changed.")
		return 1
	}

	lock, err := lockUpdates(filepath.Join(filepath.Dir(l.releaseState), "update-logs"))
	if err != nil {
		fmt.Fprintln(p.errw, err)
		return 1
	}
	// Closing is enough: the lock belongs to the open file, which a running
	// installer keeps open, so it outlives this process if that dies.
	defer lock.Close()

	want := rep.Artifacts[selfupdate.InstallerArtifact(runtime.GOARCH)]
	var done, unitChanged []string
	for i, t := range targets {
		if ctx.Err() != nil {
			fmt.Fprintln(out, "\nInterrupted before the next component was started.")
			reportProgress(out, done, targets[i:])
			return 1
		}
		if i > 0 {
			// An earlier pass can have replaced files this component shares with it.
			if c, ok := findComponent(inspectComponents(l, runtime.GOARCH, rep.Artifacts, inv), t.Component); ok && c.Status == statCurrent {
				fmt.Fprintf(out, "\n== %s: already current (the files it shares with an earlier component were updated with it); not run again\n", t.Component)
				done = append(done, t.Component)
				continue
			}
		}
		if t.nothing {
			fmt.Fprintf(out, "\n== %s: the installer has nothing left to change here; not run again\n", t.Component)
			done = append(done, t.Component)
			continue
		}
		// The installer was verified when it was downloaded; make sure the file
		// is still those bytes right before it runs as root.
		if got, err := inv.hashFile(rep.Installer); err != nil || want == "" || got != want {
			fmt.Fprintln(p.errw, "the verified installer changed on disk; stopping before it runs")
			reportProgress(out, done, targets[i:])
			return 1
		}
		logPath := filepath.Join(filepath.Dir(l.releaseState), "update-logs", fmt.Sprintf("%s-%s.log", time.Now().UTC().Format("20060102T150405Z"), t.Component))
		fmt.Fprintf(out, "\n== %s: running the signed installer (%s). If this session drops it keeps running; follow it with: tail -f %s\n", t.Component, strings.Join(t.Args, " "), logPath)
		unitPath := filepath.Join(l.unitDir, t.Unit+".service")
		unitBefore, _ := inv.readFile(unitPath)
		o := p.env.installer(ctx, installerSpec{
			Script: rep.Installer, Args: append(append([]string{}, t.Args...), "--yes"), Env: fullEnv(t),
			Detach: true, LogPath: logPath, Tee: out, Lock: lock,
		})
		if o.Err != nil || o.ExitCode != 0 {
			fmt.Fprintf(out, "\n%s FAILED: %s\nThe installer rolls back its own run; see %s.\n", t.Component, installerFailure(o), logPath)
			reportProgress(out, done, targets[i+1:])
			return 1
		}
		done = append(done, t.Component)
		if unitAfter, err := inv.readFile(unitPath); err == nil && unitBefore != nil && unitDrift(t.Component, unitBefore, unitAfter) {
			unitChanged = append(unitChanged, fmt.Sprintf("%s (%s)", t.Component, clean(unitPath)))
		}
	}

	// Trust but verify: a second plan must have nothing left to change, the
	// binaries must now be the signed release's, and the services must be
	// running those binaries, not only have them on disk. Not tied to ctx: an
	// interrupt must not turn these checks into false alarms.
	vctx := context.WithoutCancel(ctx)
	code := 0
	for _, t := range targets {
		if !contains(done, t.Component) {
			continue
		}
		pctx, pcancel := context.WithTimeout(vctx, planTimeout)
		o := p.env.installer(pctx, installerSpec{Script: rep.Installer, Args: append(append([]string{}, t.Args...), "--plan", "--json"), Env: fullEnv(t)})
		pcancel()
		plan, err := parsePlan(o.Stdout)
		if o.Err != nil || o.ExitCode != 0 || err != nil || !healthyStates[plan.State] || plan.Changes != 0 {
			fmt.Fprintf(out, "WARNING: %s still has changes pending after the update; check it with baft doctor and the installer log.\n", t.Component)
			code = 1
		}
	}
	after := inspectComponents(l, runtime.GOARCH, rep.Artifacts, inv)
	for _, c := range after {
		if contains(done, c.Name) && c.Status != statCurrent {
			fmt.Fprintf(out, "WARNING: %s does not yet run the signed release's binaries (%s).\n", c.Name, componentDetail(c))
			code = 1
		}
	}
	if !verifyRunning(out, l, done, rep.Artifacts, inv) {
		code = 1
	}
	for _, u := range unitChanged {
		fmt.Fprintf(out, "WARNING: the systemd unit of %s changed beyond what an update may change; the installer kept the previous one under %s/backups. Look at the difference before restarting anything.\n", u, clean(filepath.Dir(l.releaseState)))
		code = 1
	}
	if code == 0 {
		fmt.Fprintf(out, "\nUpdated to %s: %s. Run baft doctor to see the node's health.\n", clean(rep.Latest), strings.Join(done, ", "))
	}
	return code
}

// unitDrift says whether an update changed a unit it may not change. The one
// allowed difference is the BCC unit's record of its bootstrap script's sha256,
// which moves with the bootstrap script (see vetStagedPlan).
func unitDrift(component string, before, after []byte) bool {
	norm := func(b []byte) string {
		if component != compBCC {
			return string(b)
		}
		var keep []string
		for _, line := range strings.Split(string(b), "\n") {
			if !strings.HasPrefix(line, "# baft-bootstrap-sha256:") {
				keep = append(keep, line)
			}
		}
		return strings.Join(keep, "\n")
	}
	return norm(before) != norm(after)
}

func findComponent(comps []componentInfo, name string) (componentInfo, bool) {
	for _, c := range comps {
		if c.Name == name {
			return c, true
		}
	}
	return componentInfo{}, false
}

// runningBinary names the unit of each component and the binary that unit runs.
func runningBinary(l menuCfg, component string) (unit, binary string) {
	switch component {
	case compAgent:
		return l.agentUnit, "baft-agent"
	case compBCC:
		return l.bccService, "baft-bcc"
	}
	return l.service, "baft"
}

// verifyRunning checks that each updated service runs the new executable. A
// service that cannot be inspected (not running under systemd here, no
// /proc access) is said so, never counted as verified; one that still runs the
// previous binary fails the update, because its files look current while the
// process is not.
func verifyRunning(w io.Writer, l menuCfg, done []string, digests map[string]string, inv inventoryHost) bool {
	ok := true
	for _, name := range done {
		unit, bin := runningBinary(l, name)
		want := digests[bin+"-linux-"+runtime.GOARCH]
		if inv.runningDigest == nil || want == "" {
			continue
		}
		got, err := inv.runningDigest(unit)
		switch {
		case err != nil:
			fmt.Fprintf(w, "NOTE: could not check which binary %s is running (%s); check it with systemctl status %s.\n", clean(unit), clean(err.Error()), clean(unit))
		case got != want:
			fmt.Fprintf(w, "WARNING: %s is still running the previous %s although the file on disk is the new one; restart it: systemctl restart %s\n", clean(unit), bin, clean(unit))
			ok = false
		}
	}
	return ok
}

// sharedBinaryProblems refuses a run in which a binary is replaced under a
// BAFT transport service that is not part of the run: the file would change
// while that tunnel keeps running the old process until its next restart, and
// then silently switches to the new code. The services this covers include
// the ones the BCC tunnel builder added next to the installer's baft.service.
func sharedBinaryProblems(l menuCfg, comps []componentInfo, targets []*updateTarget, inv inventoryHost) []string {
	replaced := map[string]bool{}
	transportInRun := false
	for _, t := range targets {
		if t.Component == compTransport {
			transportInRun = true
		}
		if c, ok := findComponent(comps, t.Component); ok {
			for _, b := range c.Binaries {
				if b.State == "differs" {
					replaced[b.Path] = true
				}
			}
		}
	}
	if len(replaced) == 0 {
		return nil
	}
	runners, err := transportServices(l, inv)
	if err != nil {
		return []string{fmt.Sprintf("cannot list the services in %s to see which ones run a binary this update replaces: %v", clean(l.unitDir), err)}
	}
	var units []string
	for u := range runners {
		units = append(units, u)
	}
	sort.Strings(units)
	var out []string
	for _, u := range units {
		bin := runners[u]
		if !replaced[bin] || (u == l.service && transportInRun) {
			continue
		}
		out = append(out, fmt.Sprintf("%s runs %s, which this update replaces, but %s is not part of this run; it would keep the old process until restarted and then switch to the new code unannounced. Update it in the same run, or leave this binary alone", clean(u), clean(bin), clean(u)))
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func installerFailure(o installerOutcome) string {
	if o.Err != nil {
		return clean(o.Err.Error())
	}
	var tail []string
	for _, line := range strings.Split(o.Stderr, "\n") {
		if line = strings.TrimSpace(clean(line)); line != "" {
			tail = append(tail, line)
		}
	}
	if len(tail) > 3 {
		tail = tail[len(tail)-3:]
	}
	return fmt.Sprintf("exit %d: %s", o.ExitCode, strings.Join(tail, " | "))
}

func printBlockers(w io.Writer, blockers []string) {
	fmt.Fprintln(w, "Nothing was changed. This update cannot be applied here:")
	for _, b := range blockers {
		fmt.Fprintf(w, "  - %s\n", clean(b))
	}
	fmt.Fprintln(w, "Apply it by hand if you must: baft update check --save-installer FILE, then sudo BAFT_VERSION=vX.Y.Z bash FILE --plan with the options you installed with.")
}

func reportProgress(w io.Writer, done []string, left []*updateTarget) {
	var rest []string
	for _, t := range left {
		rest = append(rest, t.Component)
	}
	fmt.Fprintf(w, "Updated: %s. Not started: %s. Nothing else was touched.\n", listOrNone(done), listOrNone(rest))
}

func listOrNone(l []string) string {
	if len(l) == 0 {
		return "none"
	}
	return strings.Join(l, ", ")
}

func printTargetPlan(w io.Writer, t updateTarget, comps []componentInfo, latest string) {
	fmt.Fprintf(w, "[%s] unit %s, installer %s\n", t.Component, clean(t.Unit), strings.Join(t.Args, " "))
	if t.staged {
		// The installer's plan after the download is exact: show it as it is.
		var kept []string
		for _, s := range t.plan.Steps {
			if s.Action == "keep" || s.Action == "skip" {
				kept = append(kept, clean(s.Item))
				continue
			}
			fmt.Fprintf(w, "    %-22s %-8s %s\n", clean(s.Item), clean(s.Action), clean(s.Detail))
		}
		if t.nothing {
			fmt.Fprintln(w, "    nothing left to change for this component")
		}
		fmt.Fprintf(w, "    kept as they are: %s\n", strings.Join(kept, ", "))
		if hasStep(t.plan, "BCC systemd unit", "update") {
			fmt.Fprintln(w, "    (the BCC unit records the sha256 of its bootstrap script in a comment; only that line changes, and baft checks it afterwards)")
		}
		return
	}
	replaced := false
	for _, c := range comps {
		if c.Name != t.Component {
			continue
		}
		for _, b := range c.Binaries {
			if b.State == "differs" {
				replaced = true
				fmt.Fprintf(w, "    %-22s will be replaced with the signed release's (the old one is kept for rollback)\n", b.Name)
			}
		}
	}
	// The installer plans before it downloads, so by version only; a changed
	// binary always restarts its service and moves the release state forward.
	// Say so here: the confirmation must not understate a restart.
	implied := map[string]bool{}
	if replaced {
		for _, it := range []string{"service", "agent service", "BCC service", "release state"} {
			implied[it] = true
		}
		fmt.Fprintf(w, "    %-22s %-8s to run the new binary (a brief interruption of this service)\n", clean(t.Unit), "restart")
		fmt.Fprintf(w, "    %-22s %-8s move forward to %s once the update verifies\n", "release state", "update", clean(latest))
	}
	var kept []string
	for _, s := range t.plan.Steps {
		switch {
		case s.Action == "keep" || s.Action == "skip":
			if !implied[s.Item] {
				kept = append(kept, clean(s.Item))
			}
		case implied[s.Item] && strings.HasSuffix(s.Item, "service"):
			// already shown above
		default:
			fmt.Fprintf(w, "    %-22s %-8s %s\n", clean(s.Item), clean(s.Action), clean(s.Detail))
		}
	}
	fmt.Fprintf(w, "    kept as they are: %s\n", strings.Join(kept, ", "))
	if replaced {
		fmt.Fprintln(w, "    (binary steps are decided by version before the download; one identical to the signed release is kept)")
	}
}

// lockUpdates keeps two updates from running at once on this host. The lock is
// held for as long as the returned file (or a copy of it handed to the
// installer) stays open; closing the file is how it is released.
func lockUpdates(dir string) (*os.File, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("cannot create %s: %w", dir, err)
	}
	f, err := os.OpenFile(filepath.Join(dir, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, errors.New("another baft update is running on this host")
	}
	return f, nil
}

// ---- running the installer ----

type cappedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if room := b.limit - b.Len(); room > 0 {
		if len(p) > room {
			b.Buffer.Write(p[:room])
		} else {
			b.Buffer.Write(p)
		}
	}
	return len(p), nil
}

func exitCodeOf(err error) (int, error) {
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	return -1, err
}

// lineSanitizer forwards text line by line with every control character other
// than the line break and tab removed (escape sequences become inert text).
type lineSanitizer struct {
	w   io.Writer
	buf []byte
}

func (l *lineSanitizer) Write(p []byte) (int, error) {
	l.buf = append(l.buf, p...)
	for {
		i := bytes.IndexByte(l.buf, '\n')
		if i < 0 {
			break
		}
		if err := l.emit(l.buf[:i+1]); err != nil {
			return len(p), err
		}
		l.buf = l.buf[i+1:]
	}
	if len(l.buf) > 1<<16 { // a line that never ends
		err := l.emit(append(l.buf, '\n'))
		l.buf = nil
		return len(p), err
	}
	return len(p), nil
}

// Flush forwards an unfinished last line.
func (l *lineSanitizer) Flush() {
	if len(l.buf) > 0 {
		l.emit(append(l.buf, '\n'))
		l.buf = nil
	}
}

func (l *lineSanitizer) emit(line []byte) error {
	_, err := io.WriteString(l.w, strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			return r
		}
		return -1
	}, strings.ToValidUTF8(string(line), "?")))
	return err
}

// hostRunInstaller runs `bash script args` with exactly spec.Env. A plan run is
// plain and bounded by ctx. An apply run (Detach) gets its own session, no
// terminal and its output in a log file, and is NOT tied to ctx: cancelling it
// half way would skip the installer's own rollback. This process only follows
// the log; if this process dies the installer finishes on its own.
func hostRunInstaller(ctx context.Context, s installerSpec) installerOutcome {
	if bashPath == "" {
		return installerOutcome{Err: errors.New("no bash in /usr/bin or /bin (or it is writable by others); the installer cannot run")}
	}
	if !s.Detach {
		cmd := exec.CommandContext(ctx, bashPath, append([]string{s.Script}, s.Args...)...)
		cmd.Env, cmd.Dir = s.Env, "/"
		so, se := &cappedBuffer{limit: 1 << 20}, &cappedBuffer{limit: 1 << 16}
		cmd.Stdout, cmd.Stderr = so, se
		code, err := exitCodeOf(cmd.Run())
		return installerOutcome{Stdout: so.String(), Stderr: se.String(), ExitCode: code, Err: err}
	}

	if err := os.MkdirAll(filepath.Dir(s.LogPath), 0o700); err != nil {
		return installerOutcome{Err: err}
	}
	logf, err := os.OpenFile(s.LogPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return installerOutcome{Err: err}
	}
	defer logf.Close()
	cmd := exec.Command(bashPath, append([]string{s.Script}, s.Args...)...)
	cmd.Env, cmd.Dir = s.Env, "/"
	cmd.Stdout, cmd.Stderr = logf, logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if s.Lock != nil {
		cmd.ExtraFiles = []*os.File{s.Lock}
	}
	if err := cmd.Start(); err != nil {
		return installerOutcome{Err: err}
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	rd, _ := os.Open(s.LogPath)
	if rd != nil {
		defer rd.Close()
	}
	// The log keeps every byte the installer wrote; what is mirrored to the
	// terminal has its control characters stripped, because some of that text
	// (key ids, URLs) comes from files downloaded over the network.
	var tee *lineSanitizer
	if s.Tee != nil {
		tee = &lineSanitizer{w: s.Tee}
	}
	pump := func() {
		if rd != nil && tee != nil {
			io.Copy(tee, rd)
		}
	}
	tick := time.NewTicker(150 * time.Millisecond)
	defer tick.Stop()
	told := false
	for {
		select {
		case err := <-done:
			pump()
			code, rerr := exitCodeOf(err)
			return installerOutcome{ExitCode: code, Err: rerr}
		case <-tick.C:
			pump()
		case <-ctx.Done():
			if !told && s.Tee != nil {
				fmt.Fprintln(s.Tee, "\n(interrupt received: the installer keeps running to completion so that it can roll back safely)")
			}
			told = true
			ctx = context.Background() // stop selecting on the cancelled context
		}
	}
}
