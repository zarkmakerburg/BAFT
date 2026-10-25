package release_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Falsification tests for scripts/release/accept.sh: the tag it accepts must
// be the repository's current tag (never a stale local one), and CI evidence
// must come from the intended workflow file, not from a check name.

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func commit(t *testing.T, dir, name string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", name)
	git(t, dir, "commit", "-q", "-m", name)
	return git(t, dir, "rev-parse", "HEAD")
}

const fakeGH = `#!/usr/bin/env bash
# api <url>: print the fixture for that URL, fail when there is none.
url=""
for a in "$@"; do case "$a" in api|-H|Accept:*) ;; *) url="$a" ;; esac; done
f="$FIXTURES/$(printf '%s' "$url" | tr '/?&=' '____')"
[[ -f "$f" ]] && { cat "$f"; exit 0; }
echo "no fixture for $url" >&2; exit 1
`

type env struct {
	script, remote, clone, fixtures, gh string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	e := &env{
		script:   filepath.Join(root, "scripts/release/accept.sh"),
		remote:   filepath.Join(base, "remote.git"),
		clone:    filepath.Join(base, "clone"),
		fixtures: filepath.Join(base, "fx"),
		gh:       filepath.Join(base, "gh"),
	}
	os.MkdirAll(e.fixtures, 0o755)
	os.WriteFile(e.gh, []byte(fakeGH), 0o755)
	git(t, base, "init", "-q", "--bare", "-b", "main", e.remote)
	git(t, base, "clone", "-q", e.remote, e.clone)
	git(t, e.clone, "checkout", "-q", "-b", "main")
	return e
}

func (e *env) fixture(t *testing.T, url string, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	name := strings.NewReplacer("/", "_", "?", "_", "&", "_", "=", "_").Replace(url)
	if err := os.WriteFile(filepath.Join(e.fixtures, name), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func (e *env) accept(t *testing.T, remote string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", e.script, "vtest", "o/r")
	cmd.Dir = e.clone
	cmd.Env = append(os.Environ(), "ACCEPT_GH="+e.gh, "ACCEPT_REMOTE="+remote, "FIXTURES="+e.fixtures)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestStaleLocalTagIsNeverUsed(t *testing.T) {
	e := newEnv(t)
	a := commit(t, e.clone, "a")
	git(t, e.clone, "push", "-q", "origin", "main")
	git(t, e.clone, "tag", "vtest")
	git(t, e.clone, "push", "-q", "origin", "vtest")
	b := commit(t, e.clone, "b")
	git(t, e.clone, "push", "-q", "origin", "main")
	// The remote tag moves to B; the clone keeps its local tag at A.
	git(t, e.remote, "tag", "-f", "vtest", b)
	if got := git(t, e.clone, "rev-parse", "vtest^{commit}"); got != a {
		t.Fatalf("setup: local tag is %s, want stale %s", got, a)
	}

	out, _ := e.accept(t, e.remote)
	if !strings.Contains(out, "tag -> "+b) && !strings.Contains(out, "-> "+b) {
		t.Fatalf("acceptance did not bind to the remote tag (B=%s):\n%s", b, out)
	}
	if strings.Contains(out, a) {
		t.Fatalf("acceptance used the stale local tag (A=%s):\n%s", a, out)
	}
	if got := git(t, e.clone, "rev-parse", "vtest^{commit}"); got != a {
		t.Fatal("acceptance must not touch the operator's local tag")
	}

	// A failed fetch is a failure, never a fallback to the local tag.
	out, err := e.accept(t, filepath.Join(t.TempDir(), "missing.git"))
	if err == nil || !strings.Contains(out, "FAIL") || !strings.Contains(out, "cannot fetch") || strings.Contains(out, a) {
		t.Fatalf("unreachable remote: err=%v\n%s", err, out)
	}
}

type run struct {
	ID         int    `json:"id"`
	Path       string `json:"path"`
	HeadSHA    string `json:"head_sha"`
	RunNumber  int    `json:"run_number"`
	RunAttempt int    `json:"run_attempt"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

type job struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

func ciSetup(t *testing.T, runs []run, jobs map[int][]job) string {
	e := newEnv(t)
	sha := commit(t, e.clone, "a")
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

func rowFor(out, name string) string {
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, name+" ") {
			return l
		}
	}
	return ""
}

func TestCIEvidenceMustComeFromTheIntendedWorkflowFile(t *testing.T) {
	// A successful job named "test" in some OTHER workflow file (what a
	// look-alike producer could create) does not satisfy the gate.
	out := ciSetup(t,
		[]run{{ID: 90, Path: ".github/workflows/evil.yml", RunNumber: 9, Conclusion: "success"}},
		map[int][]job{90: {{ID: 1, Name: "test", Status: "completed", Conclusion: "success"}}})
	if l := rowFor(out, "ci: test"); !strings.HasPrefix(l, "FAIL") || !strings.Contains(l, "no ci.yml workflow run") {
		t.Fatalf("spoofed workflow accepted: %q\n%s", l, out)
	}
	if strings.Contains(out, "ACCEPTED") {
		t.Fatal("accepted on spoofed evidence")
	}
}

func TestRerunRuleLatestRunDecides(t *testing.T) {
	ok := []job{{ID: 1, Name: "test", Status: "completed", Conclusion: "success"}}
	bad := []job{{ID: 2, Name: "test", Status: "completed", Conclusion: "failure"}}

	// An earlier success is superseded by a later failed run.
	out := ciSetup(t,
		[]run{{ID: 11, Path: ".github/workflows/ci.yml", RunNumber: 1, Conclusion: "success"}, {ID: 12, Path: ".github/workflows/ci.yml", RunNumber: 2, Conclusion: "success"}},
		map[int][]job{11: ok, 12: bad})
	if l := rowFor(out, "ci: test"); !strings.HasPrefix(l, "FAIL") {
		t.Fatalf("a later failed run did not supersede the earlier success: %q\n%s", l, out)
	}

	// An earlier failure is superseded by a later successful re-run.
	out = ciSetup(t,
		[]run{{ID: 11, Path: ".github/workflows/ci.yml", RunNumber: 1, Conclusion: "success"}, {ID: 12, Path: ".github/workflows/ci.yml", RunNumber: 2, Conclusion: "success"}},
		map[int][]job{11: bad, 12: ok})
	if l := rowFor(out, "ci: test"); !strings.HasPrefix(l, "PASS") || !strings.Contains(l, "run #2") {
		t.Fatalf("a later successful re-run was not honoured: %q\n%s", l, out)
	}

	// A run for another commit is never evidence for this one.
	out = ciSetup(t,
		[]run{{ID: 13, Path: ".github/workflows/ci.yml", RunNumber: 5, HeadSHA: strings.Repeat("0", 40), Conclusion: "success"}},
		map[int][]job{13: ok})
	if l := rowFor(out, "ci: test"); !strings.HasPrefix(l, "FAIL") {
		t.Fatalf("a run of another commit counted: %q\n%s", l, out)
	}

	// A run that has not finished is not a pass.
	out = ciSetup(t,
		[]run{{ID: 14, Path: ".github/workflows/ci.yml", RunNumber: 1, Status: "in_progress", Conclusion: "success"}},
		map[int][]job{14: ok})
	if l := rowFor(out, "ci: test"); !strings.HasPrefix(l, "FAIL") {
		t.Fatalf("an unfinished run counted: %q\n%s", l, out)
	}
}

func qualifiedCI() ([]run, map[int][]job) {
	runs := []run{
		{ID: 21, Path: ".github/workflows/ci.yml", RunNumber: 1, Conclusion: "success"},
		{ID: 22, Path: ".github/workflows/static-analysis.yml", RunNumber: 1, Conclusion: "success"},
		{ID: 23, Path: ".github/workflows/r2-noise.yml", RunNumber: 1, Conclusion: "success"},
	}
	jobs := map[int][]job{
		22: {{ID: 22, Name: "gosec", Status: "completed", Conclusion: "success"}},
		23: {{ID: 23, Name: "r3", Status: "completed", Conclusion: "success"}},
	}
	for i, name := range strings.Fields("test release-dry-run e2e-binaries e2e-install e2e-install-release e2e-agent-enroll e2e-ssh-bootstrap e2e-launch1 third-party-licenses e2e-bcc-install e2e-bcc-schema-upgrade e2e-install-ssh e2e-install-offline e2e-uninstall installer-faults e2e-agent-bcc-versions e2e-bcc-backup-restore installer-parity") {
		jobs[21] = append(jobs[21], job{ID: 100 + i, Name: name, Status: "completed", Conclusion: "success"})
	}
	return runs, jobs
}

func TestReleaseCIRejectsUnsuccessfulWholeRun(t *testing.T) {
	for _, conclusion := range []string{"failure", "cancelled", "timed_out", "neutral", "skipped", "unknown", ""} {
		t.Run(conclusion, func(t *testing.T) {
			runs, jobs := qualifiedCI()
			runs[0].Conclusion = conclusion
			// Named jobs can all be green while another workflow job fails,
			// or the API reports an incomplete/unknown overall conclusion.
			out := ciSetup(t, runs, jobs)
			if l := rowFor(out, "ci: workflow"); !strings.HasPrefix(l, "FAIL") {
				t.Fatalf("unsuccessful whole run admitted: %q\n%s", l, out)
			}
		})
	}
}

func TestReleaseCIRequiresEveryCurrentMainJob(t *testing.T) {
	for _, missing := range strings.Fields("third-party-licenses e2e-bcc-install e2e-bcc-schema-upgrade e2e-install-ssh e2e-install-offline e2e-uninstall installer-faults e2e-agent-bcc-versions e2e-bcc-backup-restore installer-parity") {
		t.Run(missing, func(t *testing.T) {
			runs, jobs := qualifiedCI()
			for i, j := range jobs[21] {
				if j.Name == missing {
					jobs[21] = append(jobs[21][:i], jobs[21][i+1:]...)
					break
				}
			}
			out := ciSetup(t, runs, jobs)
			if l := rowFor(out, "ci: "+missing); !strings.HasPrefix(l, "FAIL") {
				t.Fatalf("missing mandatory job admitted: %q\n%s", l, out)
			}
		})
	}
}

func TestReleaseCIRequiresIndependentSecurityAndR3(t *testing.T) {
	for index, name := range map[int]string{1: "gosec", 2: "r3"} {
		for _, scenario := range []string{"missing", "wrong-sha", "wrong-workflow", "later-failure"} {
			t.Run(name+"/"+scenario, func(t *testing.T) {
				runs, jobs := qualifiedCI()
				switch scenario {
				case "missing":
					runs = append(runs[:index], runs[index+1:]...)
				case "wrong-sha":
					runs[index].HeadSHA = strings.Repeat("0", 40)
				case "wrong-workflow":
					runs[index].Path = ".github/workflows/lookalike.yml"
				case "later-failure":
					later := runs[index]
					later.ID, later.RunNumber, later.Conclusion = 99, 2, "failure"
					runs = append(runs, later)
					jobs[99] = jobs[runs[index].ID]
				}
				out := ciSetup(t, runs, jobs)
				if l := rowFor(out, "ci: "+name); !strings.HasPrefix(l, "FAIL") {
					t.Fatalf("mandatory independent gate admitted without valid evidence: %q\n%s", l, out)
				}
			})
		}
	}
}

func TestReleaseCIQualificationDoesNotReplaceArtifactAcceptance(t *testing.T) {
	runs, jobs := qualifiedCI()
	out := ciSetup(t, runs, jobs)
	for _, name := range []string{"workflow", "test", "gosec", "r3", "installer-faults", "e2e-install-offline"} {
		if l := rowFor(out, "ci: "+name); !strings.HasPrefix(l, "PASS") {
			t.Fatalf("qualified CI rejected: %q\n%s", l, out)
		}
	}
	if l := rowFor(out, "release assets"); !strings.HasPrefix(l, "FAIL") || strings.Contains(out, "ACCEPTED:") {
		t.Fatalf("green CI bypassed missing release artifact acceptance:\n%s", out)
	}
}
