package release_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// The Gamma v0.3.7 CI workflow once contained a malformed YAML literal
// that turned its Go test invocation into part of another step. YAML decode
// plus mandatory step/command checks make that regression fail at go test time.
func TestGammaV037UpgradeWorkflowSyntaxAndCommands(t *testing.T) {
	path := filepath.Join("..", "..", ".github", "workflows", "gamma-v037-upgrade-lab.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string `yaml:"name"`
				Run  string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatalf("invalid workflow YAML: %v", err)
	}
	job, ok := workflow.Jobs["upgrade-rollback"]
	if !ok {
		t.Fatal("expected upgrade-rollback job")
	}
	if len(job.Steps) < 4 {
		t.Fatalf("missing workflow steps: %d", len(job.Steps))
	}
	countVerify := 0
	countDrill := 0
	for _, step := range job.Steps {
		switch step.Name {
		case "Verify release gate rejects missing v0.3.7 upgrade evidence":
			countVerify++
			for _, must := range []string{
				"go test ./tests/release",
				"^TestGammaTaggedReleaseRequiresV037UpgradeLAB$",
				"-count=1",
				"-timeout=5m",
			} {
				if !strings.Contains(step.Run, must) {
					t.Errorf("release gate step missing %q", must)
				}
			}
		case "Validate and execute disposable v0.3.7 BCC upgrade rollback":
			countDrill++
			for _, must := range []string{
				"bash -n tests/e2e/gamma_v037_upgrade_rollback.sh",
				"bash tests/e2e/gamma_v037_upgrade_rollback.sh",
				"timeout --signal=TERM",
			} {
				if !strings.Contains(step.Run, must) {
					t.Errorf("upgrade drill step missing %q", must)
				}
			}
		}
	}
	if countVerify != 1 || countDrill != 1 {
		t.Fatalf("expected one verification and one isolated upgrade step, got %d/%d", countVerify, countDrill)
	}
}
