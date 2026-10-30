package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const sha1c = "0123456789abcdef0123456789abcdef01234567"

func loadManifest(t *testing.T) *Manifest {
	t.Helper()
	raw, err := os.ReadFile("manifest.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// jsonLog builds test2json output where every manifest test passed.
func jsonLog(m *Manifest, mutate func(e Entry) []map[string]string) string {
	var b strings.Builder
	for _, e := range m.Entries {
		evs := mutate(e)
		if evs == nil {
			evs = []map[string]string{{"Action": "run"}, {"Action": "pass"}}
		}
		for _, ev := range evs {
			ev["Package"] = e.Package
			if _, ok := ev["Test"]; !ok {
				ev["Test"] = e.Test
			} else if strings.HasPrefix(ev["Test"], "/") {
				ev["Test"] = e.Test + ev["Test"]
			}
			j, _ := json.Marshal(ev)
			b.Write(j)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func eval(t *testing.T, m *Manifest, log string) *Report {
	t.Helper()
	l, err := ParseJSON(strings.NewReader(log))
	if err != nil {
		t.Fatal(err)
	}
	return Evaluate(m, l, sha1c)
}

func statusOf(r *Report, id string) string {
	for _, row := range r.Rows {
		if row.ID == id {
			return row.Status
		}
	}
	return "?"
}

func TestManifestValidAndCoversFullPath(t *testing.T) {
	m := loadManifest(t)
	if len(m.Entries) < len(RequiredStages) {
		t.Fatal("too few entries")
	}
}

func TestManifestRejectsWeakening(t *testing.T) {
	m := loadManifest(t)
	cases := map[string]func(*Manifest){
		"drop full_path stage": func(m *Manifest) {
			var kept []Entry
			for _, e := range m.Entries {
				if e.Stage != "full_path" {
					kept = append(kept, e)
				}
			}
			m.Entries = kept
		},
		"wrong prefix (legacy blocked-path test)": func(m *Manifest) {
			m.Entries[0].Test = "TestOperationalIngressSignedPollReportsBlockedReceiptAndNeverUsesLAB"
		},
		"duplicate":        func(m *Manifest) { m.Entries = append(m.Entries, m.Entries[0]) },
		"wildcard package": func(m *Manifest) { m.Entries[0].Package = "./internal/..." },
		"regexp test name": func(m *Manifest) { m.Entries[0].Test = TestPrefix + "_A|B" },
		"unknown stage":    func(m *Manifest) { m.Entries[0].Stage = "mock" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := *m
			c.Entries = append([]Entry(nil), m.Entries...)
			mutate(&c)
			raw, _ := json.Marshal(c)
			if _, err := LoadManifest(raw); err == nil {
				t.Fatal("weakened manifest accepted")
			}
		})
	}
	if _, err := LoadManifest([]byte(`{"version":1,"name":"operational-ingress-v1","entries":[],"x":1}`)); err == nil {
		t.Fatal("unknown field accepted")
	}
}

func TestJSONAllPassIsGreen(t *testing.T) {
	m := loadManifest(t)
	r := eval(t, m, jsonLog(m, func(Entry) []map[string]string { return nil }))
	if !r.Pass || r.Passed != len(m.Entries) {
		t.Fatalf("complete PASS log rejected: %+v", r)
	}
}

func TestJSONFalsification(t *testing.T) {
	m := loadManifest(t)
	target := m.Entries[4].ID
	mut := func(f func(Entry) []map[string]string) func(Entry) []map[string]string {
		return func(e Entry) []map[string]string {
			if e.ID == target {
				return f(e)
			}
			return nil
		}
	}
	cases := []struct {
		name   string
		events []map[string]string // nil for removing the test entirely
		remove bool
		want   string
	}{
		{"missing test", nil, true, StatusMissing},
		{"run only", []map[string]string{{"Action": "run"}}, false, StatusRunOnly},
		{"skip", []map[string]string{{"Action": "run"}, {"Action": "skip"}}, false, StatusSkip},
		{"pass with skipped subtest", []map[string]string{{"Action": "run"}, {"Action": "skip", "Test": "/needs_root"}, {"Action": "pass"}}, false, StatusSkip},
		{"pass with failed subtest", []map[string]string{{"Action": "run"}, {"Action": "fail", "Test": "/x"}, {"Action": "pass"}}, false, StatusFail},
		{"fail", []map[string]string{{"Action": "run"}, {"Action": "fail"}}, false, StatusFail},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var log string
			if c.remove {
				mm := *m
				mm.Entries = nil
				for _, e := range m.Entries {
					if e.ID != target {
						mm.Entries = append(mm.Entries, e)
					}
				}
				log = jsonLog(&mm, func(Entry) []map[string]string { return nil })
			} else {
				log = jsonLog(m, mut(func(Entry) []map[string]string { return c.events }))
			}
			r := eval(t, m, log)
			if r.Pass {
				t.Fatal("gate green on falsified log")
			}
			if got := statusOf(r, target); got != c.want {
				t.Fatalf("status %s, want %s", got, c.want)
			}
		})
	}
	t.Run("package-level fail", func(t *testing.T) {
		log := jsonLog(m, func(Entry) []map[string]string { return nil })
		log += `{"Action":"fail","Package":"` + m.Entries[0].Package + `"}` + "\n"
		if eval(t, m, log).Pass {
			t.Fatal("package FAIL accepted")
		}
	})
	t.Run("same test name in another package does not count", func(t *testing.T) {
		log := jsonLog(m, func(e Entry) []map[string]string {
			if e.ID == target {
				return []map[string]string{{"Action": "run", "Package": "./other"}, {"Action": "pass", "Package": "./other"}}
			}
			return nil
		})
		// jsonLog overwrites Package; emit the other-package events manually.
		log = strings.ReplaceAll(log, `"Package":"`+m.Entries[4].Package+`","Test":"`+m.Entries[4].Test+`"`, `"Package":"./other","Test":"`+m.Entries[4].Test+`"`)
		if statusOf(eval(t, m, log), target) != StatusMissing {
			t.Fatal("foreign package PASS satisfied the manifest")
		}
	})
	t.Run("prefix name does not satisfy", func(t *testing.T) {
		log := jsonLog(m, func(e Entry) []map[string]string { return nil })
		log = strings.ReplaceAll(log, `"Test":"`+m.Entries[4].Test+`"`, `"Test":"`+m.Entries[4].Test+`Extra"`)
		if statusOf(eval(t, m, log), target) != StatusMissing {
			t.Fatal("TestXExtra satisfied TestX")
		}
	})
}

func TestCurrentInertCodeIsRed(t *testing.T) {
	// A log that contains only the existing blocked-path / contract tests and
	// no operational gate tests must never be green.
	m := loadManifest(t)
	log := `{"Action":"run","Package":"./internal/agent","Test":"TestOperationalIngressSignedPollReportsBlockedReceiptAndNeverUsesLAB"}
{"Action":"pass","Package":"./internal/agent","Test":"TestOperationalIngressSignedPollReportsBlockedReceiptAndNeverUsesLAB"}
{"Action":"pass","Package":"./internal/agent"}
`
	r := eval(t, m, log)
	if r.Pass || r.Passed != 0 {
		t.Fatalf("inert code produced green: %+v", r)
	}
}

func TestOversizedWhitespaceAndTruncationFailClosed(t *testing.T) {
	m := loadManifest(t)
	good := jsonLog(m, func(Entry) []map[string]string { return nil })
	// A huge whitespace-only line used to trip bufio.Scanner.ErrTooLong, which
	// stops scanning silently and can hide everything after it, including a SKIP.
	huge := strings.Repeat(" ", MaxLineBytes+10)
	if _, err := ParseJSON(strings.NewReader(good + huge + "\n")); err == nil {
		t.Fatal("oversized whitespace line accepted")
	}
	if _, err := ParseText(strings.NewReader("=== RUN   TestA\n"+huge+"\n--- SKIP: TestA (0.00s)\n"), "./p"); err == nil {
		t.Fatal("oversized whitespace line before a SKIP accepted in text mode")
	}
	// A normal amount of whitespace padding is fine.
	if _, err := ParseJSON(strings.NewReader(good + strings.Repeat(" ", 5000) + "\n")); err != nil {
		t.Fatalf("modest whitespace rejected: %v", err)
	}
	// Truncated mid-line JSON is rejected rather than ignored.
	if _, err := ParseJSON(strings.NewReader(good + `{"Action":"pa`)); err == nil {
		t.Fatal("truncated JSON accepted")
	}
	if _, err := ParseJSON(strings.NewReader("")); err == nil {
		t.Fatal("empty log accepted")
	}
	if _, err := ParseJSON(strings.NewReader("ok  \tpkg\t0.1s\n")); err == nil {
		t.Fatal("non-JSON log accepted in JSON mode")
	}
}

const pkgX = "./internal/x"

func textManifest(names ...string) *Manifest {
	m := &Manifest{Name: "operational-ingress-v1"}
	for _, n := range names {
		m.Entries = append(m.Entries, Entry{ID: n, Package: pkgX, Test: n})
	}
	return m
}

func evalText(t *testing.T, names []string, log string) *Report {
	t.Helper()
	l, err := ParseText(strings.NewReader(log), pkgX)
	if err != nil {
		t.Fatal(err)
	}
	return Evaluate(textManifest(names...), l, sha1c)
}

func TestTextRunAndPassSuffixHandling(t *testing.T) {
	A := "TestOperationalIngressV1Gate_A"
	pass := "=== RUN   " + A + "\n--- PASS: " + A + " (0.01s)\nPASS\nok  \tx\t0.1s\n"
	cases := []struct {
		name string
		log  string
		want string
	}{
		{"plain", pass, StatusPass},
		{"CRLF capture", strings.ReplaceAll(pass, "\n", "\r\n"), StatusPass},
		{"trailing spaces on RUN and PASS", "=== RUN   " + A + "   \n--- PASS: " + A + " (12.34s)   \n", StatusPass},
		{"long duration", "=== RUN   " + A + "\n--- PASS: " + A + " (1234.50s)\n", StatusPass},
		{"RUN only", "=== RUN   " + A + "\n", StatusRunOnly},
		{"PASS of longer name must not satisfy", "=== RUN   " + A + "Extra\n--- PASS: " + A + "Extra (0.01s)\n", StatusMissing},
		{"top-level SKIP", "=== RUN   " + A + "\n--- SKIP: " + A + " (0.00s)\n", StatusSkip},
		{"indented subtest SKIP under PASS", "=== RUN   " + A + "\n=== RUN   " + A + "/root\n    --- SKIP: " + A + "/root (0.00s)\n--- PASS: " + A + " (0.01s)\n", StatusSkip},
		{"tab-indented subtest SKIP", "=== RUN   " + A + "\n\t--- SKIP: " + A + "/root (0.00s)\n--- PASS: " + A + " (0.01s)\n", StatusSkip},
		{"subtest FAIL under PASS", "=== RUN   " + A + "\n    --- FAIL: " + A + "/x (0.00s)\n--- PASS: " + A + " (0.01s)\n", StatusFail},
		{"package FAIL line", pass + "FAIL\tx\t0.1s\n", StatusFail},
		{"panic", "=== RUN   " + A + "\npanic: boom\n", StatusRunOnly},
		{"PASS word inside test output is not a result", "=== RUN   " + A + "\n    x_test.go:1: --- PASS: " + A + " (0.00s)\n", StatusRunOnly},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := evalText(t, []string{A}, c.log)
			if got := r.Rows[0].Status; got != c.want {
				t.Fatalf("status %s, want %s\n%s", got, c.want, c.log)
			}
		})
	}
}

func TestSHAMustBeExactCommit(t *testing.T) {
	if err := CheckSHA(sha1c, sha1c); err != nil {
		t.Fatal(err)
	}
	for _, c := range [][2]string{{sha1c, strings.ToUpper(sha1c)}, {sha1c, sha1c[:39] + "8"}, {"", sha1c}, {"main", "main"}, {sha1c[:7], sha1c[:7]}} {
		if CheckSHA(c[0], c[1]) == nil {
			t.Fatalf("accepted %q vs %q", c[0], c[1])
		}
	}
}

func TestCLIExitCodes(t *testing.T) {
	m := loadManifest(t)
	dir := t.TempDir()
	good := filepath.Join(dir, "good.json")
	if err := os.WriteFile(good, []byte(jsonLog(m, func(Entry) []map[string]string { return nil })), 0o600); err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(dir, "report.json")
	var out, errb bytes.Buffer
	if code := run([]string{"-manifest", "manifest.v1.json", "-log", good, "-head-sha", sha1c, "-expect-sha", sha1c, "-out", report}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s%s", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "GATE GREEN") {
		t.Fatal(out.String())
	}
	if b, err := os.ReadFile(report); err != nil || !bytes.Contains(b, []byte(`"pass": true`)) {
		t.Fatalf("report: %v %s", err, b)
	}
	// Wrong SHA: red even with a complete log.
	other := sha1c[:39] + "8"
	if code := run([]string{"-manifest", "manifest.v1.json", "-log", good, "-head-sha", other, "-expect-sha", sha1c}, &out, &errb); code == 0 {
		t.Fatal("SHA mismatch accepted")
	}
	// Empty log (nothing ran: inert code) is red.
	empty := filepath.Join(dir, "empty.json")
	os.WriteFile(empty, nil, 0o600)
	if code := run([]string{"-manifest", "manifest.v1.json", "-log", empty, "-head-sha", sha1c, "-expect-sha", sha1c}, &out, &errb); code == 0 {
		t.Fatal("empty log accepted")
	}
	// Log with no gate tests at all is red.
	none := filepath.Join(dir, "none.json")
	os.WriteFile(none, []byte(`{"Action":"pass","Package":"./internal/bcc"}`+"\n"), 0o600)
	out.Reset()
	if code := run([]string{"-manifest", "manifest.v1.json", "-log", none, "-head-sha", sha1c, "-expect-sha", sha1c}, &out, &errb); code != 1 || !strings.Contains(out.String(), "GATE RED") {
		t.Fatalf("inert log: code %d %s", code, out.String())
	}
}

// The workflow must pin exactly this manifest and run exactly this checker.
func TestWorkflowPinsManifestAndFailsClosed(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", ".github", "workflows", "operational-ingress-v1.yml"))
	if err != nil {
		t.Fatal(err)
	}
	wf := string(raw)
	man, _ := os.ReadFile("manifest.v1.json")
	sum := sha256.Sum256(man)
	want := hex.EncodeToString(sum[:])
	m := regexp.MustCompile(`MANIFEST_SHA256:\s*([0-9a-f]{64})`).FindStringSubmatch(wf)
	if m == nil || m[1] != want {
		t.Fatalf("workflow MANIFEST_SHA256 %v does not pin manifest %s", m, want)
	}
	for _, must := range []string{"sha256sum", "go run ./tests/release/opgate", "-expect-sha", "-head-sha", "github.event.pull_request.head.sha", "if: always()"} {
		if !strings.Contains(wf, must) {
			t.Errorf("workflow missing %q", must)
		}
	}
	for _, mustNot := range []string{"continue-on-error", "|| true", "publicIngressLAB"} {
		if strings.Contains(wf, mustNot) {
			t.Errorf("workflow must not contain %q", mustNot)
		}
	}
	_ = fmt.Sprint
}
