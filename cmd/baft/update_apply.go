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
	backupFreshness = 26 * time.Hour

	exitNeedsConfirmation = 4

	installerHealthy = "ALREADY_INSTALLED"
)

// healthyStates are the installer states of an install that is present, paired
// and running. UPGRADE_AVAILABLE and CURRENT are refinements of ALREADY_INSTALLED
// (the installer only reaches them from it); every other state (FRESH_INSTALL,
// PARTIAL_INSTALL, REPAIR_REQUIRED, BROKEN_INSTALL) is not something to update.
var healthyStates = map[string]bool{installerHealthy: true, "UPGRADE_AVAILABLE": true, "CURRENT": true}

// applyOrder is control plane first, data plane last: a failure stops the run
// before the tunnels' own service is touched.
var applyOrder = []string{compBCC, compAgent, compTransport}

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
func vetPlan(p installerPlan) []string {
	var out []string
	if !healthyStates[p.State] {
		out = append(out, fmt.Sprintf("the install is %s (%s), not a healthy install to update; repair it first", clean(p.State), clean(p.Why)))
	}
	for _, s := range p.Steps {
		actions, known := allowedSteps[s.Item]
		switch {
		case !known:
			out = append(out, fmt.Sprintf("the installer plans a step this baft does not know (%q %s); apply by hand", clean(s.Item), clean(s.Action)))
		case !actions[s.Action]:
			out = append(out, fmt.Sprintf("the installer would %s %q, which an update never does", clean(s.Action), clean(s.Item)))
		}
	}
	return out
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
// from scratch: only what the installer needs from this process (paths, locale,
// proxies and certificate bundles, and the release source the check used) is
// passed on, and the downgrade switch is always off.
func installerEnv(l menuCfg, t updateTarget, version, rootPub string) []string {
	env := map[string]string{
		"BAFT_NONINTERACTIVE": "1", "BAFT_INSTALL_FROM": "release", "BAFT_VERSION": version,
		"BAFT_ALLOW_DOWNGRADE": "0", "BAFT_ROOT_PUB": rootPub,
		"BAFT_RELEASE_STATE": l.releaseState, "BAFT_PREFIX": filepath.Dir(l.releaseState), "BAFT_SYSTEMD_DIR": l.unitDir,
	}
	if env["PATH"] = os.Getenv("PATH"); env["PATH"] == "" {
		env["PATH"] = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	}
	for _, k := range []string{"HOME", "LANG", "LC_ALL", "TMPDIR", "SSL_CERT_FILE", "SSL_CERT_DIR", "CURL_CA_BUNDLE",
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
	var planOnly, yes, skipBackup bool
	var l menuCfg
	f, ok := parseOpsFlags("update apply", args, stderr, func(fs *flag.FlagSet) {
		fs.StringVar(&version, "version", "", "release to apply (default: the newest)")
		fs.StringVar(&only, "only", "", "comma-separated components: transport, agent, bcc (default: every one that is behind)")
		fs.BoolVar(&planOnly, "plan", false, "show what would change, change nothing")
		fs.BoolVar(&yes, "yes", false, "apply without asking")
		fs.BoolVar(&skipBackup, "skip-backup-check", false, "update BCC without a recent backup of it (not recommended)")
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
		layout: l, version: version, only: sel, planOnly: planOnly, yes: yes, skipBackupCheck: skipBackup,
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
	dir, err := os.MkdirTemp("", "baft-update-")
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
	fullEnv := func(t *updateTarget) []string { return installerEnv(l, *t, rep.Latest, rootText) }
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

	for _, t := range targets {
		printTargetPlan(out, *t, comps, rep.Latest)
	}
	if p.planOnly {
		fmt.Fprintln(out, "\nThis was a preview. Nothing was changed.")
		return 0
	}
	if !p.yes && !p.confirm(fmt.Sprintf("\nThis restarts the services above and moves them to %s. Type 'update' to apply: ", clean(rep.Latest))) {
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
	var done []string
	for i, t := range targets {
		// The installer was verified when it was downloaded; make sure the file
		// is still those bytes right before it runs as root.
		if got, err := inv.hashFile(rep.Installer); err != nil || want == "" || got != want {
			fmt.Fprintln(p.errw, "the verified installer changed on disk; stopping before it runs")
			reportProgress(out, done, targets[i:])
			return 1
		}
		logPath := filepath.Join(filepath.Dir(l.releaseState), "update-logs", fmt.Sprintf("%s-%s.log", time.Now().UTC().Format("20060102T150405Z"), t.Component))
		fmt.Fprintf(out, "\n== %s: running the signed installer (%s). If this session drops it keeps running; follow it with: tail -f %s\n", t.Component, strings.Join(t.Args, " "), logPath)
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
	}

	// Trust but verify: a second plan must have nothing left to change, and the
	// binaries must now be the signed release's.
	code := 0
	for _, t := range targets {
		o := p.env.installer(ctx, installerSpec{Script: rep.Installer, Args: append(append([]string{}, t.Args...), "--plan", "--json"), Env: fullEnv(t)})
		plan, err := parsePlan(o.Stdout)
		if o.Err != nil || o.ExitCode != 0 || err != nil || !healthyStates[plan.State] || plan.Changes != 0 {
			fmt.Fprintf(out, "WARNING: %s still has changes pending after the update; check it with baft doctor and the installer log.\n", t.Component)
			code = 1
		}
	}
	for _, c := range inspectComponents(l, runtime.GOARCH, rep.Artifacts, inv) {
		if contains(done, c.Name) && c.Status != statCurrent {
			fmt.Fprintf(out, "WARNING: %s does not yet run the signed release's binaries (%s).\n", c.Name, componentDetail(c))
			code = 1
		}
	}
	if code == 0 {
		fmt.Fprintf(out, "\nUpdated to %s: %s. Run baft doctor to see the node's health.\n", clean(rep.Latest), strings.Join(done, ", "))
	}
	return code
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
		return o.Err.Error()
	}
	tail := strings.Split(strings.TrimSpace(clean(o.Stderr)), "\n")
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

// hostRunInstaller runs `bash script args` with exactly spec.Env. A plan run is
// plain and bounded by ctx. An apply run (Detach) gets its own session, no
// terminal and its output in a log file, and is NOT tied to ctx: cancelling it
// half way would skip the installer's own rollback. This process only follows
// the log; if this process dies the installer finishes on its own.
func hostRunInstaller(ctx context.Context, s installerSpec) installerOutcome {
	if !s.Detach {
		cmd := exec.CommandContext(ctx, "bash", append([]string{s.Script}, s.Args...)...)
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
	cmd := exec.Command("bash", append([]string{s.Script}, s.Args...)...)
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
	pump := func() {
		if rd != nil && s.Tee != nil {
			io.Copy(s.Tee, rd)
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
