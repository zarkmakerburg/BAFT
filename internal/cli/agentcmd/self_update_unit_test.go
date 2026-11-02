package agentcmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOwnSystemdUnitFromCgroup(t *testing.T) {
	dir := t.TempDir()
	for name, c := range map[string]struct{ cgroup, want string }{
		"v2 service":    {"0::/system.slice/baft-agent.service\n", "baft-agent"},
		"v2 namespaced": {"0::/system.slice/baft-agent-rhlab.service\n", "baft-agent-rhlab"},
		"v1 hybrid":     {"12:pids:/system.slice/baft-agent.service\n1:name=systemd:/system.slice/baft-agent.service\n0::/system.slice/baft-agent.service\n", "baft-agent"},
		"login session": {"0::/user.slice/user-0.slice/session-3.scope\n", ""},
		"not a service": {"0::/\n", ""},
		"bare .service": {"0::/system.slice/.service\n", ""},
	} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(c.cgroup), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := ownSystemdUnit(p); got != c.want {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
	if got := ownSystemdUnit(filepath.Join(dir, "missing")); got != "" {
		t.Errorf("missing cgroup file: %q", got)
	}
}
