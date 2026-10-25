package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// extractShellFuncs returns the named top-level shell functions of install.sh
// exactly as shipped (from "name() {" to the first line that is just "}").
func extractShellFuncs(t *testing.T, names ...string) string {
	t.Helper()
	raw, err := os.ReadFile("../../install.sh")
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	for _, n := range names {
		re := regexp.MustCompile(`(?ms)^` + regexp.QuoteMeta(n) + `\(\) \{\n.*?^\}\n`)
		m := re.Find(raw)
		if m == nil {
			t.Fatalf("install.sh has no function %s", n)
		}
		out.Write(m)
	}
	return out.String()
}

func runInstallFuncs(t *testing.T, dir, script string) (string, error) {
	t.Helper()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "systemctl.log")
	fake := "#!/bin/sh\necho \"$*\" >> " + log + "\n"
	if err := os.WriteFile(filepath.Join(bin, "systemctl"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "-c", script)
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// The boot recovery unit must run before every BAFT service (sysinit.target
// precedes all default-dependency services, incl. each scoped instance) and
// before the two units the installer owns; it must only be ordered, never
// pull the services in, and it may never replace a foreign unit file.
func TestInstallerRecoverUnitOrdersBeforeServicesAndRefusesForeignFile(t *testing.T) {
	dir := t.TempDir()
	funcs := extractShellFuncs(t, "render_recover_unit", "install_recover_unit")
	pre := `set -eu
warn() { echo "WARN: $*" >&2; }
BAFT_BIN=/usr/local/bin/baft BAFT_CONFIG_DIR=/etc/baft BAFT_STATE_DIR=/var/lib/baft BAFT_USER=baft
BAFT_SERVICE=baft BAFT_AGENT_UNIT=baft-agent BAFT_SYSTEMD_DIR=` + dir + `/units
BAFT_RECOVER_UNIT=baft-recover.service BAFT_RECOVER_UNIT_FILE=$BAFT_SYSTEMD_DIR/$BAFT_RECOVER_UNIT
mkdir -p "$BAFT_SYSTEMD_DIR"
` + funcs
	out, err := runInstallFuncs(t, dir, pre+"install_recover_unit\n")
	if err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	unit, err := os.ReadFile(filepath.Join(dir, "units", "baft-recover.service"))
	if err != nil {
		t.Fatal(err)
	}
	u := string(unit)
	for _, want := range []string{
		"DefaultDependencies=no",
		"After=local-fs.target",
		"Before=sysinit.target baft.service baft-agent.service",
		"Type=oneshot",
		"ExecStart=/usr/local/bin/baft recover boot --config-dir /etc/baft --state-dir /var/lib/baft --unit-dir " + dir + "/units --bin-dir /usr/local/bin --service baft --service-user baft",
		"WantedBy=sysinit.target",
		"# baft-component: recover",
	} {
		if !strings.Contains(u, want) {
			t.Errorf("unit lacks %q:\n%s", want, u)
		}
	}
	for _, bad := range []string{"Requires=", "Wants=baft", "RequiredBy=", "systemctl", "restart"} {
		if strings.Contains(u, bad) {
			t.Errorf("recovery unit must only be ordered, found %q:\n%s", bad, u)
		}
	}
	log, _ := os.ReadFile(filepath.Join(dir, "systemctl.log"))
	if !strings.Contains(string(log), "enable baft-recover.service") {
		t.Fatalf("unit not enabled: %s", log)
	}
	// Idempotent.
	if out, err := runInstallFuncs(t, dir, pre+"install_recover_unit\n"); err != nil {
		t.Fatalf("rerun: %v\n%s", err, out)
	}
	again, _ := os.ReadFile(filepath.Join(dir, "units", "baft-recover.service"))
	if string(again) != u {
		t.Fatal("rerun rewrote the unit")
	}
	// A foreign unit of that name is never replaced.
	foreign := filepath.Join(dir, "units", "baft-recover.service")
	if err := os.WriteFile(foreign, []byte("[Service]\nExecStart=/bin/true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = runInstallFuncs(t, dir, pre+"install_recover_unit\n")
	if err != nil || !strings.Contains(out, "not BAFT's") {
		t.Fatalf("foreign file case: %v %s", err, out)
	}
	if b, _ := os.ReadFile(foreign); !strings.Contains(string(b), "/bin/true") {
		t.Fatal("foreign unit was replaced")
	}
}
