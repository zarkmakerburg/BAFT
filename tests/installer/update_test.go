package installer_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const managedPanelUnit = "[Unit]\n# baft-managed: true\n# baft-component: bcc\n[Service]\nExecStart=/x/baft-bcc --listen 127.0.0.1:18090 --state-file y\n"

func (r *planRig) withPanel() { r.write("sysd/baft-bcc.service", managedPanelUnit, 0o644) }

func (r *planRig) withAgent() {
	r.write("sysd/baft-agent.service", "[Service]\nExecStart=/x/baft-agent --bcc-url https://bcc.example.com --node-id upd-1 --token-file t --interval 7s --metrics-listen 127.0.0.1:9191\n", 0o644)
	r.write("agent/token", "tok\n", 0o600)
	r.write("agent/bcc-job.pub", "JOBKEY\n", 0o644)
}

// update runs install.sh with args and returns its combined output and error.
func (r *planRig) update(args ...string) (string, error) {
	r.t.Helper()
	cmd := exec.Command("bash", append([]string{filepath.Join("..", "..", "install.sh")}, args...)...)
	cmd.Env = r.env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func before(t *testing.T, out, first, second string) {
	t.Helper()
	i, j := strings.Index(out, first), strings.Index(out, second)
	if i < 0 || j < 0 || i > j {
		t.Fatalf("want %q before %q:\n%s", first, second, out)
	}
}

// --update finds the panel and the agent, plans the panel first, and reuses
// the agent's enrollment (token, pinned job key) instead of asking for it.
func TestUpdateFindsEveryComponentPanelFirst(t *testing.T) {
	r := newPlanRig(t)
	r.withPanel()
	r.withAgent()
	out, err := r.update("--update", "--plan")
	if err != nil {
		t.Fatalf("--update --plan: %v\n%s", err, out)
	}
	before(t, out, "found: panel", "found: agent")
	before(t, out, "updating panel", "updating agent")
	for _, want := range []string{"BCC binary", "baft-agent binary", "nothing was changed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	for _, item := range []string{"agent token", "BCC job key"} {
		found := false
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, item) && strings.Contains(line, " keep ") {
				found = true
			}
		}
		if !found {
			t.Fatalf("the agent update must keep the installed %s:\n%s", item, out)
		}
	}
	if strings.Contains(out, "What is this server for") {
		t.Fatalf("an update asked what the server is for:\n%s", out)
	}
}

// A plain rerun on a server that already runs BAFT updates it rather than
// asking for a role; with nothing installed it still asks.
func TestPlainRerunOnInstalledServerUpdates(t *testing.T) {
	r := newPlanRig(t)
	r.withPanel()
	out, err := r.update("--plan")
	if err != nil || !strings.Contains(out, "found: panel") || strings.Contains(out, "What is this server for") {
		t.Fatalf("plain rerun did not take the update path: err=%v\n%s", err, out)
	}
}

func TestUpdateRefusals(t *testing.T) {
	r := newPlanRig(t)
	if out, err := r.update("--update", "--plan"); err == nil || !strings.Contains(out, "nothing to update") {
		t.Fatalf("empty server: err=%v\n%s", err, out)
	}
	r.withPanel()
	for _, args := range [][]string{{"--role", "ex"}, {"--bcc-domain", "x.example.com"}, {"--re-pair"}} {
		out, err := r.update(append([]string{"--update", "--plan"}, args...)...)
		if err == nil || !strings.Contains(out, "takes only") {
			t.Fatalf("--update %v accepted: err=%v\n%s", args, err, out)
		}
	}
	if out, err := r.update("--update", "--plan", "--json"); err == nil || !strings.Contains(out, "drop --json") {
		t.Fatalf("--update --json accepted: err=%v\n%s", err, out)
	}
	// A panel unit this installer did not write cannot keep its address
	// automatically; the update stops instead of guessing.
	r.write("sysd/baft-bcc.service", "[Service]\nExecStart=/x/baft-bcc --listen 0.0.0.0:443\n", 0o644)
	if out, err := r.update("--update", "--plan"); err == nil || !strings.Contains(out, "was not written by this installer") {
		t.Fatalf("unmanaged panel unit: err=%v\n%s", err, out)
	}
}

// A CLI tunnel is updated by its role; a tunnel BCC deployed on an agent
// server belongs to BCC and is covered by the agent's binaries.
func TestUpdateTunnelRole(t *testing.T) {
	r := newPlanRig(t)
	r.installed(true)
	r.write("etc/baft.yaml", "{\"server\": {\"listen\": \"0.0.0.0:443\"}}\n", 0o640)
	out, err := r.update("--update", "--plan")
	if err != nil || !strings.Contains(out, "found: tunnel, outside Iran") {
		t.Fatalf("EX tunnel not found: err=%v\n%s", err, out)
	}
	r.withAgent()
	out, err = r.update("--update", "--plan")
	if err != nil || strings.Contains(out, "found: tunnel") || !strings.Contains(out, "found: agent") {
		t.Fatalf("on an agent server the tunnel must not be updated by role: err=%v\n%s", err, out)
	}
	r2 := newPlanRig(t)
	r2.installed(true)
	if out, err := r2.update("--update", "--plan"); err == nil || !strings.Contains(out, "not recognisable") {
		t.Fatalf("an unrecognisable config must stop the update: err=%v\n%s", err, out)
	}
}
