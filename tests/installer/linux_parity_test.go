package installer_test

// Installer / Linux-script parity (v0.4.0 gate row 5).
//
// install.sh is the only installer. Its output has to stay identical to what
// the Go code that later manages the same host writes (tunnelnode, used by
// `baft tunnel` and by BCC-driven tunnel jobs through baft-agent) and has to
// use options the shipped binaries actually accept. These tests run install.sh's
// own shell functions and diff the result; any drift fails the build.

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/zarkmakerburg/baft/internal/tunnelnode"
)

const repoRoot = "../.."

type nullSystem struct{}

func (nullSystem) Systemctl(context.Context, ...string) (string, error)   { return "", nil }
func (nullSystem) Run(context.Context, string, ...string) (string, error) { return "", nil }

func needBash(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("installer parity requires Linux")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not installed")
	}
}

func installScript(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot, "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// shellFunc extracts one top-level function from install.sh and runs it with
// exactly the given environment.
func shellFunc(t *testing.T, fn string, env map[string]string) string {
	t.Helper()
	var body []string
	in := false
	for _, line := range strings.Split(installScript(t), "\n") {
		if !in {
			if line != fn+"() {" {
				continue
			}
			in = true
		}
		body = append(body, line)
		if line == "}" {
			break
		}
	}
	if len(body) == 0 || body[len(body)-1] != "}" {
		t.Fatalf("%s not found in install.sh", fn)
	}
	path := filepath.Join(t.TempDir(), fn+".sh")
	if err := os.WriteFile(path, []byte(strings.Join(body, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "-c", `set -euo pipefail; . "$1"; `+fn, "bash", path)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s: %v\n%s", fn, err, stderr.String())
	}
	return string(out)
}

// dropBlank removes blank lines. install.sh renders an empty capability
// variable as a blank line inside [Service]; systemd ignores it. Nothing else
// is normalised: order, spelling and every directive are compared exactly.
func dropBlank(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

func lineDiff(got, want []string) string {
	var b strings.Builder
	n := len(got)
	if len(want) > n {
		n = len(want)
	}
	for i := 0; i < n; i++ {
		var g, w string
		if i < len(got) {
			g = got[i]
		}
		if i < len(want) {
			w = want[i]
		}
		if g != w {
			fmt.Fprintf(&b, "line %d:\n  go: %q\n  sh: %q\n", i+1, g, w)
		}
	}
	return b.String()
}

func newGoManager(t *testing.T) *tunnelnode.Manager {
	t.Helper()
	m, err := tunnelnode.New(tunnelnode.Env{
		ConfigDir: "/etc/baft", StateDir: "/var/lib/baft", UnitDir: t.TempDir(),
		Service: "baft", User: "baft",
		BaftBin: "/usr/local/bin/baft", PairBin: "/usr/local/bin/baft-pair",
		System: nullSystem{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// The transport unit written by install.sh and the one tunnelnode writes when
// BCC or `baft tunnel` rebuilds the same service must be the same unit, so a
// host moves between the two paths without a spurious unit change.
func TestParityTransportUnitInstallShVsTunnelnode(t *testing.T) {
	needBash(t)
	m := newGoManager(t)
	for _, port := range []int{80, 443, 1023, 1024, 8443, 18443} {
		sh := shellFunc(t, "render_service_unit", map[string]string{
			"BAFT_USER": "baft", "BAFT_BIN": "/usr/local/bin/baft", "CONFIG": "/etc/baft/baft.yaml",
			"BAFT_STATE_DIR": "/var/lib/baft", "BAFT_PORT": fmt.Sprint(port),
		})
		var goUnit []string
		for _, l := range dropBlank(m.ManagedUnit(port, "tun-1", 1)) {
			// Ownership markers are tunnelnode's own header; install.sh's
			// markers are written by the installer around the render.
			if !strings.HasPrefix(l, "# baft-") {
				goUnit = append(goUnit, l)
			}
		}
		if d := lineDiff(goUnit, dropBlank(sh)); d != "" {
			t.Errorf("port %d: tunnelnode unit differs from install.sh render_service_unit:\n%s", port, d)
		}
		low := port < 1024
		if low != strings.Contains(sh, "CAP_NET_BIND_SERVICE") {
			t.Errorf("port %d: install.sh capability handling wrong (low=%v)", port, low)
		}
	}
}

// helpFlags returns every flag a binary accepts and its default.
func helpFlags(t *testing.T, bin string, args ...string) map[string]string {
	t.Helper()
	cmd := exec.Command(bin, append(args, "-h")...)
	out, _ := cmd.CombinedOutput() // -h exits non-zero by design
	flags := map[string]string{}
	name := ""
	def := regexp.MustCompile(`\(default (.*)\)\s*$`)
	for _, l := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(l, "  -") {
			f := strings.Fields(strings.TrimPrefix(l, "  -"))
			name = f[0]
			flags[name] = ""
			continue
		}
		if name != "" {
			if m := def.FindStringSubmatch(l); m != nil {
				flags[name] = strings.Trim(m[1], `"`)
			}
		}
	}
	if len(flags) == 0 {
		t.Fatalf("%s %v -h listed no flags:\n%s", bin, args, out)
	}
	return flags
}

func buildBin(t *testing.T, dir, pkg string) string {
	t.Helper()
	out := filepath.Join(dir, filepath.Base(pkg))
	cmd := exec.Command("go", "build", "-o", out, pkg)
	cmd.Dir = repoRoot
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", pkg, err, b)
	}
	return out
}

func execFlags(unit string) []string {
	var flags []string
	for _, l := range strings.Split(unit, "\n") {
		if !strings.HasPrefix(l, "ExecStart=") {
			continue
		}
		for _, f := range strings.Fields(l) {
			if strings.HasPrefix(f, "--") {
				flags = append(flags, strings.TrimPrefix(f, "--"))
			}
		}
	}
	return flags
}

// Every option install.sh puts in a unit's ExecStart must be accepted by the
// shipped binary, and every installer default must equal the binary default.
func TestParityInstallerOptionsAndDefaultsMatchBinaries(t *testing.T) {
	needBash(t)
	dir := t.TempDir()
	agentBin := buildBin(t, dir, "./cmd/baft-agent")
	bccBin := buildBin(t, dir, "./cmd/baft-bcc")
	baftBin := buildBin(t, dir, "./cmd/baft")

	agent := helpFlags(t, agentBin)
	bccf := helpFlags(t, bccBin)

	agentEnv := map[string]string{
		"BAFT_ROOT_PUB": "x", "BAFT_BCC_URL": "http://127.0.0.1:1", "BAFT_AGENT_RELEASE_BASE_URL": "http://r",
		"BAFT_AGENT_REVOCATIONS_URL": "http://r/rev", "BAFT_BIN": "/usr/local/bin/baft",
		"BAFT_AGENT_BIN": "/usr/local/bin/baft-agent", "BAFT_NODE_ID": "n1", "BAFT_AGENT_DIR": "/etc/baft-agent",
		"BAFT_AGENT_STATE_DIR": "/var/lib/baft-agent", "BAFT_RELEASE_STATE": "/opt/baft/release-state.json",
		"BAFT_SERVICE": "baft", "BAFT_CONFIG_DIR": "/etc/baft", "BAFT_STATE_DIR": "/var/lib/baft",
		"BAFT_USER": "baft", "BAFT_METRICS_LISTEN": "127.0.0.1:9191", "BAFT_AGENT_INTERVAL": "30s",
		"BAFT_PREFIX": "/opt/baft", "BAFT_SYSTEMD_DIR": "/etc/systemd/system",
	}
	agentUnit := shellFunc(t, "render_agent_unit", agentEnv)
	for _, f := range execFlags(agentUnit) {
		if _, ok := agent[f]; !ok {
			t.Errorf("install.sh gives baft-agent --%s, which baft-agent does not define", f)
		}
	}
	// Defaults an operator relies on when the unit omits a flag.
	wantAgent := map[string]string{
		"token-file": "/etc/baft-agent/token", "bcc-job-key": "/etc/baft-agent/bcc-job.pub",
		"release-root": "/etc/baft-agent/release-root.pub", "state-dir": "/var/lib/baft-agent",
		"release-state": "/opt/baft/release-state.json", "bin-dir": "/usr/local/bin", "service": "baft",
		"config-dir": "/etc/baft", "baft-state-dir": "/var/lib/baft", "unit-dir": "/etc/systemd/system",
		"metrics-listen": "127.0.0.1:9191", "service-user": "baft", "interval": "30s",
	}
	for k, v := range wantAgent {
		if agent[k] != v {
			t.Errorf("baft-agent --%s default %q != installer default %q", k, agent[k], v)
		}
	}

	bccUnit := shellFunc(t, "render_bcc_unit", map[string]string{
		"BAFT_BCC_LISTEN": "203.0.113.5:8443", "BAFT_BCC_TLS_CERT": "/c.pem", "BAFT_BCC_TLS_KEY": "/k.pem",
		"BAFT_BCC_ALLOW_INSECURE_HTTP": "1", "BAFT_BCC_BOOTSTRAP_SCRIPT": "/opt/baft/bcc-bootstrap/install.sh",
		"BAFT_BCC_PUBLIC_URL": "https://bcc.example.com", "BAFT_BCC_BIN": "/usr/local/bin/baft-bcc",
		"BAFT_BCC_STATE_FILE": "/var/lib/baft-bcc/bcc-state.json", "BAFT_BCC_ADMIN_TOKEN_FILE": "/etc/baft-bcc/admin-token",
		"BAFT_BCC_ACCESS_FILE": "/etc/baft-bcc/access.json", "BAFT_BCC_JOB_KEY_FILE": "/etc/baft-bcc/job-key",
		"BAFT_BCC_BACKUP_DIR": "/var/lib/baft-bcc/backups", "BAFT_BCC_STATE_DIR": "/var/lib/baft-bcc",
	})
	for _, f := range execFlags(bccUnit) {
		if _, ok := bccf[f]; !ok {
			t.Errorf("install.sh gives baft-bcc --%s, which baft-bcc does not define", f)
		}
	}
	if !strings.Contains(bccUnit, "--backup-dir /var/lib/baft-bcc/backups") {
		t.Errorf("BCC unit lost its --backup-dir")
	}

	// `baft run --file` (transport unit) and the `baft tunnel` defaults.
	if _, ok := helpFlags(t, baftBin, "run")["file"]; !ok {
		t.Errorf("`baft run` has no --file, which render_service_unit passes")
	}
	tun := helpFlags(t, baftBin, "tunnel", "inspect")
	for k, v := range map[string]string{
		"config-dir": "/etc/baft", "state-dir": "/var/lib/baft", "unit-dir": "/etc/systemd/system",
		"bin-dir": "/usr/local/bin", "service-user": "baft",
	} {
		if tun[k] != v {
			t.Errorf("baft tunnel --%s default %q != installer default %q", k, tun[k], v)
		}
	}
	unin := helpFlags(t, baftBin, "uninstall")
	for k, v := range map[string]string{
		"config-dir": "/etc/baft", "state-dir": "/var/lib/baft", "bin-dir": "/usr/local/bin",
		"agent-dir": "/etc/baft-agent", "agent-state-dir": "/var/lib/baft-agent",
	} {
		if unin[k] != v {
			t.Errorf("baft uninstall --%s default %q != installer default %q", k, unin[k], v)
		}
	}

	// The installer defaults themselves (guards the table above against a
	// change on the install.sh side only).
	sh := installScript(t)
	for k, v := range map[string]string{
		"BAFT_CONFIG_DIR": "/etc/baft", "BAFT_STATE_DIR": "/var/lib/baft", "BAFT_BIN": "/usr/local/bin/baft",
		"BAFT_PAIR_BIN": "/usr/local/bin/baft-pair", "BAFT_USER": "baft", "BAFT_SERVICE": "baft",
		"BAFT_SYSTEMD_DIR": "/etc/systemd/system", "BAFT_AGENT_DIR": "/etc/baft-agent",
		"BAFT_AGENT_STATE_DIR": "/var/lib/baft-agent", "BAFT_AGENT_BIN": "/usr/local/bin/baft-agent",
		"BAFT_METRICS_LISTEN": "127.0.0.1:9191", "BAFT_AGENT_INTERVAL": "30s", "BAFT_PREFIX": "/opt/baft",
	} {
		re := regexp.MustCompile(`(?m)^` + k + `="\$\{` + k + `:-([^}]*)\}"$`)
		m := re.FindStringSubmatch(sh)
		if m == nil || m[1] != v {
			t.Errorf("install.sh default for %s is %v, contract says %q", k, m, v)
		}
	}
}

// Permissions and owners: install.sh vs the constants tunnelnode writes with.
func TestParityPermissionsInstallShVsTunnelnode(t *testing.T) {
	sh := installScript(t)
	goSrc, err := os.ReadFile(filepath.Join(repoRoot, "internal/tunnelnode/tunnelnode.go"))
	if err != nil {
		t.Fatal(err)
	}
	g := string(goSrc)
	type rule struct{ name, shRe, goRe string }
	for _, r := range []rule{
		{"unit file 0644", `chmod 0644 "\$UNIT_FILE"`, `writeFile\(m\.unitPath\(\), \[\]byte\(want\), 0o644\)`},
		{"config file 0640 root:service-user", `chmod 0640 "\$CONFIG"`, `writeFile\(tmp, b, 0o640\)`},
		{"config dir 0750 root:service-user", `install -d -m 0750 -o root -g "\$BAFT_USER" "\$BAFT_CONFIG_DIR"`, `os\.MkdirAll\(m\.ConfigDir, 0o750\)`},
		{"state dir 0700 service-user", `install -d -m 0700 -o "\$BAFT_USER" -g "\$BAFT_USER" "\$BAFT_STATE_DIR"`, `os\.Chmod\(path, 0o700\)`},
	} {
		if !regexp.MustCompile(r.shRe).MatchString(sh) {
			t.Errorf("%s: install.sh no longer contains %s", r.name, r.shRe)
		}
		if !regexp.MustCompile(r.goRe).MatchString(g) {
			t.Errorf("%s: tunnelnode.go no longer contains %s", r.name, r.goRe)
		}
	}
	// Credentials the installer creates stay root-only.
	for _, want := range []string{
		`install -d -m 0700 -o root -g root "$BAFT_AGENT_DIR"`,
		`install -d -m 0700 -o root -g root "$BAFT_BCC_CONFIG_DIR"`,
		`install -d -m 0700 -o root -g root "$BAFT_BCC_STATE_DIR"`,
	} {
		if !strings.Contains(sh, want) {
			t.Errorf("install.sh no longer contains %q", want)
		}
	}
}

// The installer shipped in a release, pushed by BCC over SSH and kept as the
// BCC bootstrap script is one and the same file: build.sh may not transform it.
func TestParityReleaseInstallerIsInstallShVerbatim(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot, "scripts/release/build.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`install -m 0755 install\.sh "\$out/baft-install-linux-\$arch"`).Match(b) {
		t.Fatal("build.sh no longer ships install.sh verbatim as baft-install-linux-$arch")
	}
	rel, err := os.ReadFile(filepath.Join(repoRoot, ".github/workflows/release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, arch := range []string{"amd64", "arm64"} {
		if !strings.Contains(string(rel), "dist/baft-install-linux-"+arch) {
			t.Errorf("release.yml no longer compares install.sh with the %s installer", arch)
		}
	}
	// The managed BCC bootstrap script is a byte copy of the installer in use.
	if !strings.Contains(installScript(t), `BAFT_BCC_BOOTSTRAP_SCRIPT`) {
		t.Error("install.sh lost the BCC bootstrap script")
	}
}
