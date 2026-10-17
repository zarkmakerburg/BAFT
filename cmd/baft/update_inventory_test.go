package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zarkmakerburg/baft/internal/selfupdate"
	"github.com/zarkmakerburg/baft/internal/uninstall"
)

// invRig is a host with the installer's real units and binaries in a temp
// directory.
type invRig struct {
	t      *testing.T
	dir    string
	layout menuCfg
	body   map[string]string // binary base name -> file content
}

func hexSum(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func newInvRig(t *testing.T) *invRig {
	t.Helper()
	r := &invRig{t: t, dir: t.TempDir(), body: map[string]string{}}
	r.layout = menuCfg{
		file: filepath.Join(r.dir, "etc", "baft.yaml"), service: "baft", releaseState: filepath.Join(r.dir, "opt", "release-state.json"),
		unitDir: filepath.Join(r.dir, "units"), stateDir: filepath.Join(r.dir, "var"), agentUnit: "baft-agent", bccService: "baft-bcc",
	}
	if err := os.MkdirAll(r.layout.unitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return r
}

func (r *invRig) write(path, content string) {
	r.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		r.t.Fatal(err)
	}
}

func (r *invRig) bin(name, content string) string {
	p := filepath.Join(r.dir, "bin", name)
	r.write(p, content)
	r.body[name] = content
	return p
}

func (r *invRig) unit(name, content string) {
	r.write(filepath.Join(r.layout.unitDir, name+".service"), content)
}

func (r *invRig) transport() {
	bin := r.bin("baft", "baft-v1")
	r.bin("baft-pair", "pair-v1")
	r.unit(r.layout.service, uninstall.InstallerServiceUnit("baft", bin, filepath.Join(r.dir, "etc", "baft.yaml"), r.layout.stateDir, false))
}

func (r *invRig) agent() {
	agentBin := r.bin("baft-agent", "agent-v1")
	binDir := filepath.Dir(agentBin)
	r.unit(r.layout.agentUnit, uninstall.InstallerAgentUnit(uninstall.AgentUnitSettings{
		AgentBin: agentBin, BCCURL: "https://bcc.example.com", NodeID: "n1", AgentDir: filepath.Join(r.dir, "agent"),
		ReleaseRoot: true, AgentStateDir: filepath.Join(r.dir, "ags"), ReleaseState: r.layout.releaseState, BinDir: binDir,
		Service: r.layout.service, ConfigDir: filepath.Join(r.dir, "etc"), BaftStateDir: r.layout.stateDir,
		User: "baft", MetricsListen: "127.0.0.1:9191", Interval: "30s",
		Prefix: filepath.Dir(r.layout.releaseState), SystemdDir: r.layout.unitDir,
	}))
	r.bin("baft", "baft-v1")
	r.bin("baft-pair", "pair-v1")
}

// bcc writes a BCC unit shaped like install.sh's render_bcc_unit.
func (r *invRig) bcc(labelled bool) {
	bin := r.bin("baft-bcc", "bcc-v1")
	head := ""
	if labelled {
		head = "# baft-managed: true\n# baft-component: bcc\n# baft-bootstrap-script: /opt/baft/bcc-bootstrap/install.sh\n# baft-bootstrap-sha256: absent\n"
	}
	r.unit(r.layout.bccService, head+"[Unit]\nDescription=BAFT Command Center\n\n[Service]\nType=simple\nExecStart="+bin+" --listen 127.0.0.1:8080 --state-file /var/lib/baft-bcc/state.db\nRestart=on-failure\n")
}

// digests are the signed sha256s of a release whose binaries are `bodies`.
func digests(bodies map[string]string) map[string]string {
	d := map[string]string{}
	for name, content := range bodies {
		d[name+"-linux-"+runtime.GOARCH] = hexSum(content)
	}
	return d
}

func (r *invRig) inspect(d map[string]string) []componentInfo {
	return inspectComponents(r.layout, runtime.GOARCH, d, hostInventory)
}

func byName(cs []componentInfo, name string) componentInfo {
	for _, c := range cs {
		if c.Name == name {
			return c
		}
	}
	return componentInfo{}
}

func TestEveryComponentOfAnUpToDateHostIsCurrent(t *testing.T) {
	r := newInvRig(t)
	r.transport()
	r.agent()
	r.bcc(true)
	cs := r.inspect(digests(map[string]string{"baft": "baft-v1", "baft-pair": "pair-v1", "baft-agent": "agent-v1", "baft-bcc": "bcc-v1"}))
	if len(cs) != 3 {
		t.Fatalf("components = %+v", cs)
	}
	for _, c := range cs {
		if c.Status != statCurrent {
			t.Errorf("%s: %+v", c.Name, c)
		}
	}
	if got := len(byName(cs, compAgent).Binaries); got != 3 {
		t.Errorf("the agent judges %d binaries, want baft-agent + baft + baft-pair", got)
	}
}

func TestCoreCurrentButBCCBehindIsCaught(t *testing.T) {
	// The production case: the shared release state already says v0.3.4 and
	// Core was updated, but the panel still runs the old binary.
	r := newInvRig(t)
	r.transport()
	r.bcc(true)
	cs := r.inspect(digests(map[string]string{"baft": "baft-v1", "baft-pair": "pair-v1", "baft-bcc": "bcc-v2"}))
	if byName(cs, compTransport).Status != statCurrent {
		t.Errorf("transport: %+v", byName(cs, compTransport))
	}
	bcc := byName(cs, compBCC)
	if bcc.Status != statOutdated || bcc.Binaries[0].State != "differs" {
		t.Fatalf("bcc: %+v", bcc)
	}

	rep := selfupdate.Report{Status: selfupdate.Current, Installed: "v0.3.4", Latest: "v0.3.4"}
	foldInventory(&rep, cs)
	if rep.Status != selfupdate.UpdateAvailable || !strings.Contains(rep.Reason, "bcc") || strings.Contains(rep.Reason, "transport") {
		t.Fatalf("report = %+v", rep)
	}
}

func TestFoldInventoryChangesOnlyACurrentVerdict(t *testing.T) {
	behind := []componentInfo{{Name: compBCC, Status: statOutdated}}
	for _, st := range []selfupdate.Status{selfupdate.UpdateAvailable, selfupdate.Unknown} {
		rep := selfupdate.Report{Status: st, Reason: "kept"}
		foldInventory(&rep, behind)
		if rep.Status != st || rep.Reason != "kept" {
			t.Errorf("%s verdict changed: %+v", st, rep)
		}
	}
	rep := selfupdate.Report{Status: selfupdate.Current}
	foldInventory(&rep, []componentInfo{{Name: compBCC, Status: statUnknown}, {Name: compTransport, Status: statCurrent}})
	if rep.Status != selfupdate.Current {
		t.Errorf("an unknown component alone turned a current host into an update: %+v", rep)
	}
}

func TestNothingIsClaimedWithoutAVerifiedRelease(t *testing.T) {
	r := newInvRig(t)
	r.transport()
	cs := r.inspect(nil)
	c := byName(cs, compTransport)
	if c.Status != statUnknown || !strings.Contains(c.Reason, "no verified release") || c.Binaries[0].State != "unverifiable" {
		t.Fatalf("component = %+v", c)
	}
}

func TestForeignUnitsAndMissingBinariesAreUnknownNotCurrent(t *testing.T) {
	r := newInvRig(t)
	r.unit(r.layout.service, "[Service]\nExecStart=/usr/local/bin/baft run --file /etc/baft/baft.yaml\n")
	r.bcc(false) // no installer labels
	cs := r.inspect(digests(map[string]string{"baft": "x", "baft-bcc": "bcc-v1"}))
	for _, name := range []string{compTransport, compBCC} {
		c := byName(cs, name)
		if c.Status != statUnknown || len(c.Binaries) != 0 || !strings.Contains(c.Reason, "install.sh writes") {
			t.Errorf("%s: %+v", name, c)
		}
	}

	r2 := newInvRig(t)
	r2.transport()
	if err := os.Remove(filepath.Join(r2.dir, "bin", "baft-pair")); err != nil {
		t.Fatal(err)
	}
	c := byName(r2.inspect(digests(map[string]string{"baft": "baft-v1", "baft-pair": "pair-v1"})), compTransport)
	if c.Status != statUnknown || !strings.Contains(c.Reason, "cannot be read") {
		t.Fatalf("a missing binary: %+v", c)
	}
}

func TestAnAbsentComponentIsNotListed(t *testing.T) {
	r := newInvRig(t)
	r.transport()
	cs := r.inspect(digests(map[string]string{"baft": "baft-v1", "baft-pair": "pair-v1"}))
	if len(cs) != 1 || cs[0].Name != compTransport {
		t.Fatalf("components = %+v", cs)
	}
}

// ---- through the command and the menu ----

func TestUpdateCheckReportsABehindComponentOnACurrentRelease(t *testing.T) {
	r := newInvRig(t)
	r.transport()
	r.bcc(true)
	h := newFakeHost(t)
	h.env.inventory = &hostInventory
	rep := reportCurrent
	rep.Artifacts = digests(map[string]string{"baft": "baft-v1", "baft-pair": "pair-v1", "baft-bcc": "bcc-v2"})
	h.env.checkUpdate = func(context.Context, string, string, string) selfupdate.Report { return rep }

	args := []string{"check", "--unit-dir", r.layout.unitDir, "--service", r.layout.service, "--release-state", r.layout.releaseState}
	code, out, errOut := runUpdateArgs(t, h, args...)
	if code != exitUpdateAvailable || errOut != "" {
		t.Fatalf("code=%d stderr=%q\n%s", code, errOut, out)
	}
	for _, w := range []string{"UPDATE AVAILABLE", "components    judged by their own binaries", "transport  baft", "CURRENT   all binaries identical", "bcc        baft-bcc", "OUTDATED  baft-bcc differs from the signed release", "run binaries that differ from the signed release: bcc"} {
		if !strings.Contains(out, w) {
			t.Errorf("output lacks %q:\n%s", w, out)
		}
	}

	code, out, _ = runUpdateArgs(t, h, append(args, "--json")...)
	var got struct {
		Status     string          `json:"status"`
		Components []componentInfo `json:"components"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || code != exitUpdateAvailable {
		t.Fatalf("code=%d err=%v\n%s", code, err, out)
	}
	if got.Status != "update_available" || byName(got.Components, compBCC).Status != statOutdated || byName(got.Components, compTransport).Status != statCurrent {
		t.Fatalf("json = %+v", got)
	}
}

func TestUpdateCheckOnAHostWithNoComponentsPrintsNoComponentBlock(t *testing.T) {
	h := newFakeHost(t) // its inventory finds nothing
	(&updateStub{rep: reportCurrent}).install(h)
	_, out, _ := runUpdateArgs(t, h, "check")
	if strings.Contains(out, "components") {
		t.Fatalf("a component block for a host without components:\n%s", out)
	}
}
