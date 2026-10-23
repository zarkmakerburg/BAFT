package release_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// Dynamic protection means preserving whichever real port a pre-existing
// service uses. A green workflow that exercises only a literal 2053 is
// insufficient acceptance evidence.
func TestGammaExistingServicePortMatrixIsExecutable(t *testing.T) {
	cases := []struct {
		script   string
		workflow string
		job      string
	}{
		{"gamma_two_node_namespace.sh", "gamma-two-node-lab.yml", "isolated-two-node"},
		{"gamma_v037_tunnel_upgrade_namespace.sh", "gamma-v037-tunnel-upgrade-lab.yml", "v037-two-node-upgrade"},
	}
	for _, tc := range cases {
		t.Run(tc.script, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join("..", "e2e", tc.script))
			if err != nil { t.Fatal(err) }
			script := string(src)
			guard := strings.Index(script, "GAMMA_EXISTING_PORT:-2053")
			mutation := strings.Index(script, "ip netns add")
			if guard < 0 || mutation < 0 || guard >= mutation {
				t.Fatal("existing-service port validation must precede network mutations")
			}
			for _, marker := range []string{
				"EXISTING_PORT", "existing service port conflicts with LAB fixtures",
				"http://10.221.52.2:${EXISTING_PORT}/",
			} {
				if !strings.Contains(script, marker) { t.Fatalf("dynamic port handling missing %q", marker) }
			}
			if strings.Contains(script, "http://10.221.52.2:2053/") {
				t.Fatal("actual service check still pinned to port 2053")
			}
			b, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", tc.workflow))
			if err != nil { t.Fatal(err) }
			var flow struct {
				Jobs map[string]struct {
					Steps []struct {
						Name string `yaml:"name"`
						Run string `yaml:"run"`
					} `yaml:"steps"`
				} `yaml:"jobs"`
			}
			if err := yaml.Unmarshal(b, &flow); err != nil { t.Fatalf("invalid workflow: %v", err) }
			job, ok := flow.Jobs[tc.job]
			if !ok { t.Fatalf("missing %s job", tc.job) }
			var runs string
			for _, step := range job.Steps { runs += step.Run + "\n" }
			for _, required := range []string{
				"bash -n tests/e2e/" + tc.script,
				"sudo bash tests/e2e/" + tc.script,
				"GAMMA_EXISTING_PORT=24443 bash tests/e2e/" + tc.script,
				"GAMMA_EXISTING_PORT=18443 bash tests/e2e/" + tc.script,
				"test \"$(ip netns list)\" = \"$before\"",
			} {
				if !strings.Contains(runs, required) {
					t.Errorf("hosted acceptance lacks command %q", required)
				}
			}
		})
	}
}
