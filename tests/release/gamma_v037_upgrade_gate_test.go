package release_test

import (
    "os"
    "path/filepath"
    "strconv"
    "strings"
    "testing"
)

// A tagged ingress-capable release must prove the separate two-namespace
// acceptance workflow on the exact tagged SHA, not merely its generic CI jobs.
// These fixtures use disposable git repositories and a fake gh API.
func TestGammaTaggedReleaseRequiresV037UpgradeLAB(t *testing.T) {
    for _, scenario := range []string{"missing", "lookalike", "failed", "running", "later-failed", "green"} {
        t.Run(scenario, func(t *testing.T) {
            e := newEnv(t)
            wf := filepath.Join(e.clone, ".github", "workflows")
            if err := os.MkdirAll(wf, 0o755); err != nil { t.Fatal(err) }
            if err := os.WriteFile(filepath.Join(wf, "gamma-v037-upgrade-lab.yml"),
                []byte("name: Gamma v0.3.7 Upgrade Rollback LAB\n"), 0o644); err != nil { t.Fatal(err) }
            git(t, e.clone, "add", ".github/workflows/gamma-v037-upgrade-lab.yml")
            git(t, e.clone, "commit", "-q", "-m", "add two-node LAB")
            sha := git(t, e.clone, "rev-parse", "HEAD")
            git(t, e.clone, "push", "-q", "origin", "main")
            git(t, e.clone, "tag", "vtest")
            git(t, e.clone, "push", "-q", "origin", "vtest")
            runs, jobs := qualifiedCI()
            gate := run{ID: 25, Path: ".github/workflows/gamma-v037-upgrade-lab.yml",
                RunNumber: 1, Status: "completed", Conclusion: "success"}
            jobs[25] = []job{{ID: 250, Name: "upgrade-rollback", Status: "completed", Conclusion: "success"}}
            switch scenario {
            case "missing":
                // The workflow file exists in the tag, but no run exists.
            case "lookalike":
                gate.Path = ".github/workflows/lookalike.yml"
                runs = append(runs, gate)
            case "failed":
                gate.Conclusion = "failure"
                runs = append(runs, gate)
            case "running":
                gate.Status = "in_progress"
                runs = append(runs, gate)
            case "later-failed":
                runs = append(runs, gate)
                newer := gate
                newer.ID, newer.RunNumber, newer.Conclusion = 26, 2, "failure"
                jobs[26] = jobs[25]
                runs = append(runs, newer)
            case "green":
                runs = append(runs, gate)
            }
            for i := range runs { runs[i].HeadSHA = sha }
            e.fixture(t, "repos/o/r/actions/runs?head_sha="+sha+"&per_page=100",
                map[string]any{"workflow_runs": runs})
            for id, js := range jobs {
                e.fixture(t, "repos/o/r/actions/runs/"+strconv.Itoa(id)+"/jobs?filter=latest&per_page=100",
                    map[string]any{"jobs": js})
            }
            out, _ := e.accept(t, e.remote)
            got := rowFor(out, "ci: gamma-v037-upgrade-lab")
            want := "FAIL"
            if scenario == "green" { want = "PASS" }
            if !strings.HasPrefix(got, want) {
                t.Fatalf("%s: expected %s for exact-SHA two-node LAB, got %q\n%s", scenario, want, got, out)
            }
        })
    }
}
