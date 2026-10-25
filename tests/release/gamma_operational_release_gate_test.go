package release_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestGammaOperationalWorkflowRequiresPositiveRunner(t *testing.T) {
	raw, err := os.ReadFile("../../.github/workflows/operational-ingress-lab.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Run             string
				ContinueOnError bool `yaml:"continue-on-error"`
			}
		}
	}
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatal(err)
	}
	job, ok := workflow.Jobs["operational-ingress"]
	if !ok {
		t.Fatal("missing operational-ingress job")
	}
	found := 0
	for _, step := range job.Steps {
		if strings.TrimSpace(step.Run) == "bash tests/e2e/gamma_operational_ingress_lab.sh" {
			if step.ContinueOnError {
				t.Fatal("positive acceptance may not ignore failure")
			}
			found++
		}
	}
	if found != 1 {
		t.Fatalf("want one mandatory positive runner, got %d", found)
	}
}

// These are release-evidence selection fixtures, not simulated ingress success.
func TestGammaTaggedReleaseRequiresOperationalIngressLAB(t *testing.T) {
	for _, scenario := range []string{"missing", "lookalike", "wrong-sha", "failed", "running", "skipped-job", "later-failed", "green"} {
		t.Run(scenario, func(t *testing.T) {
			e := newEnv(t)
			wf := ".github/workflows/operational-ingress-lab.yml"
			path := filepath.Join(e.clone, wf)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("name: Operational Ingress LAB\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			git(t, e.clone, "add", wf)
			git(t, e.clone, "commit", "-q", "-m", "add operational gate")
			sha := git(t, e.clone, "rev-parse", "HEAD")
			git(t, e.clone, "push", "-q", "origin", "main")
			git(t, e.clone, "tag", "vtest")
			git(t, e.clone, "push", "-q", "origin", "vtest")
			runs, jobs := qualifiedCI()
			gate := run{ID: 28, Path: wf, HeadSHA: sha, RunNumber: 1, Status: "completed", Conclusion: "success"}
			jobs[28] = []job{{ID: 280, Name: "operational-ingress", Status: "completed", Conclusion: "success"}}
			switch scenario {
			case "missing":
			case "lookalike":
				gate.Path = ".github/workflows/lookalike.yml"
			case "wrong-sha":
				gate.HeadSHA = strings.Repeat("0", 40)
			case "failed":
				gate.Conclusion = "failure"
			case "running":
				gate.Status = "in_progress"
			case "skipped-job":
				jobs[28][0].Conclusion = "skipped"
			case "later-failed":
				runs = append(runs, gate)
				gate.ID, gate.RunNumber, gate.Conclusion = 29, 2, "failure"
				jobs[29] = jobs[28]
			}
			if scenario != "missing" {
				runs = append(runs, gate)
			}
			for i := range runs {
				if runs[i].HeadSHA == "" {
					runs[i].HeadSHA = sha
				}
				if runs[i].Status == "" {
					runs[i].Status = "completed"
				}
			}
			e.fixture(t, "repos/o/r/actions/runs?head_sha="+sha+"&per_page=100", map[string]any{"workflow_runs": runs})
			for id, js := range jobs {
				e.fixture(t, "repos/o/r/actions/runs/"+strconv.Itoa(id)+"/jobs?filter=latest&per_page=100", map[string]any{"jobs": js})
			}
			out, _ := e.accept(t, e.remote)
			if row := rowFor(out, "ci: workflow"); !strings.HasPrefix(row, "PASS") {
				t.Fatalf("fixture did not establish green generic CI: %s", out)
			}
			want := "FAIL"
			if scenario == "green" {
				want = "PASS"
			}
			if row := rowFor(out, "ci: operational-ingress-lab"); !strings.HasPrefix(row, want) {
				t.Fatalf("want %s operational gate, got %q\n%s", want, row, out)
			}
		})
	}
}

func TestGammaOperationalLogRequiresExactPositiveExecution(t *testing.T) {
	const pkg = "github.com/zarkmakerburg/baft/internal/bcc"
	const name = "TestOperationalIngressFullLifecycleLAB"
	type event struct{ Action, Package, Test string }
	base := []event{{"run", pkg, name}, {"pass", pkg, name}, {"pass", pkg, ""}}
	for _, scenario := range []string{"valid", "empty", "run-only", "skip", "subtest-skip", "suffix", "other-package", "duplicate", "invalid-json"} {
		t.Run(scenario, func(t *testing.T) {
			events := append([]event(nil), base...)
			switch scenario {
			case "empty":
				events = nil
			case "run-only":
				events = events[:1]
			case "skip":
				events[1].Action = "skip"
			case "subtest-skip":
				events = append(events, event{"skip", pkg, name + "/restart"})
			case "suffix":
				events[0].Test += "Fake"
				events[1].Test += "Fake"
			case "other-package":
				for i := range events {
					events[i].Package = "other"
				}
			case "duplicate":
				events = append(events, base[1])
			}
			var log strings.Builder
			for _, e := range events {
				data, err := json.Marshal(e)
				if err != nil {
					t.Fatal(err)
				}
				log.Write(data)
				log.WriteByte('\n')
			}
			if scenario == "invalid-json" {
				log.WriteString("not json\n")
			}
			file := filepath.Join(t.TempDir(), "test.json")
			if err := os.WriteFile(file, []byte(log.String()), 0o600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("bash", "../e2e/gamma_operational_log_assert.sh", file)
			out, err := cmd.CombinedOutput()
			if (err == nil) != (scenario == "valid") {
				t.Fatalf("%s: err=%v output=%s", scenario, err, out)
			}
		})
	}
}
