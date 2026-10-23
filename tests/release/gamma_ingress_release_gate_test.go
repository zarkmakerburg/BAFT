package release_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// gammaIngressGate runs the published-tag acceptance script against a disposable
// local repository which includes the v0.4.0 ingress kernel workflow. No host
// resources or public GitHub actions are mutated.
func gammaIngressGate(t *testing.T, scenario string) string {
	t.Helper()
	e := newEnv(t)
	path := filepath.Join(e.clone, ".github", "workflows")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "p0-ingress-lab.yml"), []byte("name: P0 Public Ingress LAB\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sha := commit(t, e.clone, "gamma")
	git(t, e.clone, "add", ".github/workflows/p0-ingress-lab.yml")
	git(t, e.clone, "commit", "-q", "-m", "add ingress lab workflow")
	sha = git(t, e.clone, "rev-parse", "HEAD")
	git(t, e.clone, "push", "-q", "origin", "main")
	git(t, e.clone, "tag", "v0.4.0")
	git(t, e.clone, "push", "-q", "origin", "v0.4.0")

	runs, jobs := qualifiedCI()
	kernel := run{ID: 24, Path: ".github/workflows/p0-ingress-lab.yml", RunNumber: 1, Conclusion: "success"}
	switch scenario {
	case "missing":
		// A green CI workflow is not proof that the real kernel LAB ran.
	case "lookalike":
		kernel.Path = ".github/workflows/pretend-ingress-lab.yml"
		runs = append(runs, kernel)
	case "failed":
		kernel.Conclusion = "failure"
		runs = append(runs, kernel)
		jobs[24] = []job{{ID: 240, Name: "lab-acceptance", Status: "completed", Conclusion: "success"}}
	case "green":
		runs = append(runs, kernel)
		jobs[24] = []job{{ID: 240, Name: "lab-acceptance", Status: "completed", Conclusion: "success"}}
	default:
		t.Fatalf("unknown scenario %q", scenario)
	}
	for i := range runs {
		runs[i].HeadSHA = sha
	}
	e.fixture(t, "repos/o/r/actions/runs?head_sha="+sha+"&per_page=100", map[string]any{"workflow_runs": runs})
	for id, js := range jobs {
		e.fixture(t, "repos/o/r/actions/runs/"+strconv.Itoa(id)+"/jobs?filter=latest&per_page=100", map[string]any{"jobs": js})
	}
	cmd := exec.Command("bash", e.script, "v0.4.0", "o/r")
	cmd.Dir = e.clone
	cmd.Env = append(os.Environ(), "ACCEPT_GH="+e.gh, "ACCEPT_REMOTE="+e.remote, "FIXTURES="+e.fixtures)
	out, _ := cmd.CombinedOutput()
	return string(out)
}

func TestGammaReleaseGateRequiresRealIngressKernelLAB(t *testing.T) {
	for _, scenario := range []string{"missing", "lookalike", "failed", "green"} {
		t.Run(scenario, func(t *testing.T) {
			out := gammaIngressGate(t, scenario)
			row := rowFor(out, "ci: ingress-kernel-lab")
			want := "FAIL"
			if scenario == "green" {
				want = "PASS"
			}
			if !strings.HasPrefix(row, want) {
				t.Fatalf("kernel LAB gate %s: want %s, got %q\n%s", scenario, want, row, out)
			}
		})
	}
}
