package release_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ciSetupFiles is ciSetup for a candidate whose tagged commit carries the given
// files (workflow files that make a conditional gate apply).
func ciSetupFiles(t *testing.T, files []string, runs []run, jobs map[int][]job) string {
	t.Helper()
	e := newEnv(t)
	for _, f := range files {
		p := filepath.Join(e.clone, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("name: x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(e.clone, "a"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, e.clone, "add", "-A")
	git(t, e.clone, "commit", "-q", "-m", "candidate")
	sha := git(t, e.clone, "rev-parse", "HEAD")
	git(t, e.clone, "push", "-q", "origin", "main")
	git(t, e.clone, "tag", "vtest")
	git(t, e.clone, "push", "-q", "origin", "vtest")
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
		e.fixture(t, fmt.Sprintf("repos/o/r/actions/runs/%d/jobs?filter=latest&per_page=100", id), map[string]any{"jobs": js})
	}
	out, _ := e.accept(t, e.remote)
	return out
}

const opWorkflow = ".github/workflows/operational-ingress-v1.yml"

func withOperational(runs []run, jobs map[int][]job, conclusion string) ([]run, map[int][]job) {
	runs = append(runs, run{ID: 31, Path: opWorkflow, RunNumber: 1, Conclusion: conclusion})
	jobs[31] = []job{{ID: 310, Name: "operational-ingress-v1", Status: "completed", Conclusion: conclusion}}
	return runs, jobs
}

// The operational ingress v1 gate is required for every tag, including a tag
// whose commit does not even carry the workflow file (omission must not pass).
func TestReleaseCIRequiresOperationalIngressUnconditionally(t *testing.T) {
	for _, carriesFile := range []bool{true, false} {
		name := "workflow-file-absent-at-tag"
		var files []string
		if carriesFile {
			name = "workflow-file-present-at-tag"
			files = []string{opWorkflow}
		}
		t.Run(name+"/no-run", func(t *testing.T) {
			runs, jobs := qualifiedCI()
			out := ciSetupFiles(t, files, runs, jobs)
			if l := rowFor(out, "ci: operational-ingress-v1"); !strings.HasPrefix(l, "FAIL") || !strings.Contains(l, "no "+opWorkflow) {
				t.Fatalf("tag accepted without an operational-ingress-v1 run: %q\n%s", l, out)
			}
			if strings.Contains(out, "ACCEPTED:") {
				t.Fatal("accepted without operational ingress evidence")
			}
		})
		t.Run(name+"/green", func(t *testing.T) {
			runs, jobs := qualifiedCI()
			runs, jobs = withOperational(runs, jobs, "success")
			out := ciSetupFiles(t, files, runs, jobs)
			if l := rowFor(out, "ci: operational-ingress-v1"); !strings.HasPrefix(l, "PASS") {
				t.Fatalf("green operational gate rejected: %q\n%s", l, out)
			}
		})
	}
}

func TestReleaseCIOperationalIngressFalsification(t *testing.T) {
	for _, scenario := range []string{"failure", "cancelled", "wrong-sha", "wrong-workflow", "wrong-job", "later-failure", "unfinished"} {
		t.Run(scenario, func(t *testing.T) {
			runs, jobs := qualifiedCI()
			conclusion := "success"
			if scenario == "failure" || scenario == "cancelled" {
				conclusion = scenario
			}
			runs, jobs = withOperational(runs, jobs, conclusion)
			op := len(runs) - 1
			switch scenario {
			case "wrong-sha":
				runs[op].HeadSHA = strings.Repeat("0", 40)
			case "wrong-workflow":
				runs[op].Path = ".github/workflows/lookalike.yml"
			case "wrong-job":
				jobs[31][0].Name = "something-else"
			case "unfinished":
				runs[op].Status = "in_progress"
			case "later-failure":
				later := runs[op]
				later.ID, later.RunNumber, later.Conclusion = 32, 2, "failure"
				runs = append(runs, later)
				jobs[32] = []job{{ID: 320, Name: "operational-ingress-v1", Status: "completed", Conclusion: "failure"}}
			}
			out := ciSetupFiles(t, []string{opWorkflow}, runs, jobs)
			if l := rowFor(out, "ci: operational-ingress-v1"); !strings.HasPrefix(l, "FAIL") || strings.Contains(out, "ACCEPTED:") {
				t.Fatalf("operational gate admitted on %s: %q\n%s", scenario, l, out)
			}
		})
	}
}

// Gates that become required once their workflow file is part of the tagged
// commit: a missing or red run is a FAIL, a green run passes.
func TestReleaseCIRequiresCarriedSecurityUIAndContinuityGates(t *testing.T) {
	gates := []struct{ path, job, label string }{
		{".github/workflows/bcc-v3-ui-tests.yml", "test", "bcc-ui-unit"},
		{".github/workflows/bcc-v3-ui-tests.yml", "browser", "bcc-ui-browser"},
		{".github/workflows/vulncheck.yml", "govulncheck", "vulncheck"},
		{".github/workflows/delta-ingress-pin.yml", "check", "delta-ingress-pin"},
		{".github/workflows/operational-recovery.yml", "operational-recovery", "operational-recovery"},
		{".github/workflows/shared-operational-ingress.yml", "shared-operational-ingress", "shared-operational-ingress"},
	}
	for _, g := range gates {
		for _, scenario := range []string{"absent-file-not-required", "missing-run", "failed", "green"} {
			t.Run(g.label+"/"+scenario, func(t *testing.T) {
				runs, jobs := qualifiedCI()
				runs, jobs = withOperational(runs, jobs, "success")
				files := []string{opWorkflow}
				if scenario != "absent-file-not-required" {
					files = append(files, g.path)
				}
				if scenario == "failed" || scenario == "green" {
					c := "success"
					if scenario == "failed" {
						c = "failure"
					}
					runs = append(runs, run{ID: 41, Path: g.path, RunNumber: 1, Conclusion: c})
					jobs[41] = []job{{ID: 410, Name: g.job, Status: "completed", Conclusion: c}}
				}
				out := ciSetupFiles(t, files, runs, jobs)
				l := rowFor(out, "ci: "+g.label)
				switch scenario {
				case "absent-file-not-required":
					if l != "" {
						t.Fatalf("gate applied to a tag that does not carry the workflow: %q", l)
					}
				case "green":
					if !strings.HasPrefix(l, "PASS") {
						t.Fatalf("green gate rejected: %q\n%s", l, out)
					}
				default:
					if !strings.HasPrefix(l, "FAIL") || strings.Contains(out, "ACCEPTED:") {
						t.Fatalf("%s admitted: %q\n%s", scenario, l, out)
					}
				}
			})
		}
	}
}

// Green transport and ingress runs from another source, a lookalike workflow,
// or an older successful attempt cannot stand in for the tagged candidate's
// combined lifecycle and crash-recovery evidence.
func TestReleaseCISharedAndRecoveryRejectUnqualifiedEvidence(t *testing.T) {
	for _, g := range []struct{ path, job string }{
		{".github/workflows/operational-recovery.yml", "operational-recovery"},
		{".github/workflows/shared-operational-ingress.yml", "shared-operational-ingress"},
	} {
		for _, scenario := range []string{"wrong-sha", "wrong-workflow", "wrong-job", "later-failure", "later-attempt-failure", "unfinished-workflow", "unfinished-job", "failed-job"} {
			t.Run(g.job+"/"+scenario, func(t *testing.T) {
				runs, jobs := qualifiedCI()
				runs, jobs = withOperational(runs, jobs, "success")
				runs = append(runs, run{ID: 41, Path: g.path, RunNumber: 1, RunAttempt: 1, Conclusion: "success"})
				jobs[41] = []job{{ID: 410, Name: g.job, Status: "completed", Conclusion: "success"}}
				current := len(runs) - 1
				switch scenario {
				case "wrong-sha":
					runs[current].HeadSHA = strings.Repeat("0", 40)
				case "wrong-workflow":
					runs[current].Path = ".github/workflows/lookalike.yml"
				case "wrong-job":
					jobs[41][0].Name = "something-else"
				case "later-failure", "later-attempt-failure":
					later := runs[current]
					later.ID, later.Conclusion = 42, "failure"
					if scenario == "later-attempt-failure" {
						later.RunAttempt = 2
					} else {
						later.RunNumber = 2
					}
					runs = append(runs, later)
					jobs[42] = []job{{ID: 420, Name: g.job, Status: "completed", Conclusion: "failure"}}
				case "unfinished-workflow":
					runs[current].Status = "in_progress"
				case "unfinished-job":
					jobs[41][0].Status, jobs[41][0].Conclusion = "in_progress", ""
				case "failed-job":
					jobs[41][0].Conclusion = "failure"
				}
				out := ciSetupFiles(t, []string{opWorkflow, g.path}, runs, jobs)
				if l := rowFor(out, "ci: "+g.job); !strings.HasPrefix(l, "FAIL") || strings.Contains(out, "ACCEPTED:") {
					t.Fatalf("continuity gate admitted on %s: %q\n%s", scenario, l, out)
				}
			})
		}
	}
}

// Every workflow gate accept.sh names must exist in this repository with the
// named job, so a rename cannot silently turn a gate into a permanent FAIL or
// leave it pointing at a job that no longer exists.
func TestAcceptShNamesRealWorkflowJobs(t *testing.T) {
	root, _ := filepath.Abs("../..")
	script, err := os.ReadFile(filepath.Join(root, "scripts/release/accept.sh"))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, l := range strings.Split(string(script), "\n") {
		l = strings.TrimSpace(l)
		if !strings.HasPrefix(l, "\".github/workflows/") {
			continue
		}
		parts := strings.Split(strings.Trim(l, "\""), "|")
		if len(parts) != 3 {
			t.Fatalf("malformed gate %q", l)
		}
		n++
		wf, err := os.ReadFile(filepath.Join(root, parts[0]))
		if err != nil {
			t.Fatalf("accept.sh names a missing workflow: %v", err)
		}
		if !strings.Contains(string(wf), "\n  "+parts[1]+":\n") {
			t.Errorf("%s has no job %q", parts[0], parts[1])
		}
	}
	if n != 11 {
		t.Fatalf("expected 11 workflow gates in accept.sh, found %d", n)
	}
}
