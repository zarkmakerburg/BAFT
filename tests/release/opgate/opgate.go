// Package main is the "operational ingress v1" release-gate policy checker.
//
// It reads the output of the pinned required tests (go test -json, or a
// single-package go test -v text log) and succeeds ONLY when every test named
// in the pinned manifest has a real top-level PASS, with zero SKIP and zero
// FAIL anywhere beneath it. Missing tests, RUN-only tests, package failures,
// truncated or oversized input all fail closed.
//
// The checker never decides what to require from the log itself (the older
// family-by-RUN-line approach let a deleted test disappear silently); the
// required set comes from the manifest, whose SHA-256 the workflow pins.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
)

const (
	// MaxLogBytes bounds the whole log; MaxLineBytes bounds one line. Hitting
	// either is an error, never a silent truncation that could hide a SKIP.
	MaxLogBytes  = 256 << 20
	MaxLineBytes = 4 << 20

	TestPrefix = "TestOperationalIngressV1Gate"
)

// RequiredStages is the full path the gate must cover. A manifest missing any
// stage is invalid, so the gate cannot be weakened by deleting entries.
var RequiredStages = []string{
	"wizard_reviewed_plan",   // reviewed BCC Wizard API (plan hash, ConfigPort==Port)
	"signed_normal_agent",    // normal enrolled Agent polling, signed job admission
	"held_lease",             // OS-held ingressauthority lease from signed job + trusted policy
	"listener_nft",           // real listener + nft in a private netns
	"client_tcp",             // client TCP bytes through the public ingress
	"https_receipt",          // Agent -> BCC authenticated HTTPS receipt
	"bcc_verified_card",      // BCC Verified + deployable config/card only after receipt
	"full_path",              // single end-to-end test chaining all of the above
	"negative_identity",      // wrong node/plan/hash/port/lease/generation/enrollment/TLS
	"negative_replay_expiry", // replay and expiry
	"negative_foreign",       // active/stopped foreign services incl. v4/v6 sibling
	"recovery",               // partial Add/Remove, interruption, restart, Retire
	"siblings",               // sibling tunnels survive install/rollback; multi-EX on one IR
	"cli_pairing",            // CLI pairing with inaccessible BCC and re-pair
}

type Entry struct {
	ID      string `json:"id"`
	Stage   string `json:"stage"`
	Package string `json:"package"`
	Test    string `json:"test"`
}

type Manifest struct {
	Version int     `json:"version"`
	Name    string  `json:"name"`
	Entries []Entry `json:"entries"`
}

var (
	testNameRe = regexp.MustCompile(`^Test[A-Za-z0-9_]+$`)
	pkgRe      = regexp.MustCompile(`^\./[A-Za-z0-9_./-]+$`)
	shaRe      = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// LoadManifest strictly decodes and validates the manifest.
func LoadManifest(raw []byte) (*Manifest, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("manifest: trailing data")
	}
	if m.Version != 1 || m.Name != "operational-ingress-v1" {
		return nil, errors.New("manifest: wrong version or name")
	}
	seenID, seenTest := map[string]bool{}, map[string]bool{}
	stages := map[string]bool{}
	for _, e := range m.Entries {
		if e.ID == "" || seenID[e.ID] {
			return nil, fmt.Errorf("manifest: empty or duplicate id %q", e.ID)
		}
		seenID[e.ID] = true
		if !testNameRe.MatchString(e.Test) || !strings.HasPrefix(e.Test, TestPrefix) {
			return nil, fmt.Errorf("manifest: test %q must match %s*", e.Test, TestPrefix)
		}
		if !pkgRe.MatchString(e.Package) || strings.Contains(e.Package, "..") || strings.Contains(e.Package, "...") {
			return nil, fmt.Errorf("manifest: bad package %q", e.Package)
		}
		key := e.Package + "\x00" + e.Test
		if seenTest[key] {
			return nil, fmt.Errorf("manifest: duplicate test %s %s", e.Package, e.Test)
		}
		seenTest[key] = true
		stages[e.Stage] = true
	}
	for _, s := range RequiredStages {
		if !stages[s] {
			return nil, fmt.Errorf("manifest: required stage %q has no test", s)
		}
	}
	for s := range stages {
		known := false
		for _, r := range RequiredStages {
			known = known || r == s
		}
		if !known {
			return nil, fmt.Errorf("manifest: unknown stage %q", s)
		}
	}
	return &m, nil
}

// Result statuses.
const (
	StatusPass    = "PASS"
	StatusMissing = "MISSING" // test never ran (absent, or package did not build)
	StatusRunOnly = "RUN-ONLY"
	StatusSkip    = "SKIP"
	StatusFail    = "FAIL"
)

type Observed struct {
	Ran, Passed, Failed bool
	Skipped             []string // the test or any subtest that skipped
	FailedSub           []string
}

type Log struct {
	// Tests keyed by package + "\x00" + top-level test name.
	Tests map[string]*Observed
	// FailedPackages holds packages with a package-level fail event.
	FailedPackages map[string]bool
}

func key(pkg, test string) string { return pkg + "\x00" + test }

func (l *Log) obs(pkg, name string) *Observed {
	top := name
	if i := strings.IndexByte(name, '/'); i >= 0 {
		top = name[:i]
	}
	k := key(pkg, top)
	o := l.Tests[k]
	if o == nil {
		o = &Observed{}
		l.Tests[k] = o
	}
	return o
}

func newLog() *Log { return &Log{Tests: map[string]*Observed{}, FailedPackages: map[string]bool{}} }

// readLines splits r into lines without bufio.Scanner's silent-stop failure
// mode: an oversized line (for example a huge run of whitespace) or an oversized
// log is an error.
func readLines(r io.Reader, fn func(line string) error) error {
	br := bufio.NewReaderSize(r, 64<<10)
	total := 0
	var cur []byte
	for {
		chunk, isPrefix, err := br.ReadLine()
		if err != nil && err != io.EOF {
			return err
		}
		total += len(chunk)
		if total > MaxLogBytes {
			return fmt.Errorf("log exceeds %d bytes", MaxLogBytes)
		}
		if len(cur)+len(chunk) > MaxLineBytes {
			return fmt.Errorf("line exceeds %d bytes", MaxLineBytes)
		}
		cur = append(cur, chunk...)
		if err == io.EOF {
			if len(cur) > 0 {
				return fn(string(cur))
			}
			return nil
		}
		if isPrefix {
			continue
		}
		if e := fn(string(cur)); e != nil {
			return e
		}
		cur = cur[:0]
	}
}

type jsonEvent struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
}

// ParseJSON parses `go test -json` / test2json output.
func ParseJSON(r io.Reader) (*Log, error) {
	l := newLog()
	n := 0
	err := readLines(r, func(line string) error {
		if strings.TrimSpace(line) == "" {
			return nil
		}
		n++
		var ev jsonEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return fmt.Errorf("line %d: not test2json: %w", n, err)
		}
		if ev.Package == "" {
			return fmt.Errorf("line %d: event without package", n)
		}
		if ev.Test == "" {
			if ev.Action == "fail" {
				l.FailedPackages[ev.Package] = true
			}
			return nil
		}
		o := l.obs(ev.Package, ev.Test)
		sub := strings.Contains(ev.Test, "/")
		switch ev.Action {
		case "run":
			o.Ran = true
		case "pass":
			if !sub {
				o.Passed = true
			}
		case "skip":
			o.Skipped = append(o.Skipped, ev.Test)
		case "fail":
			if sub {
				o.FailedSub = append(o.FailedSub, ev.Test)
			} else {
				o.Failed = true
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errors.New("empty log")
	}
	return l, nil
}

// go test -v result lines. Subtest results are indented; the CR from a CRLF
// capture and any amount of trailing whitespace are tolerated; the name is an
// exact token, so TestFoo never matches TestFooBar and a "(0.01s)" suffix is
// never part of the name.
var (
	textResultRe = regexp.MustCompile(`^[ \t]*--- (PASS|FAIL|SKIP): (\S+) \(\d+(?:\.\d+)?s\)[ \t\r]*$`)
	textRunRe    = regexp.MustCompile(`^[ \t]*=== RUN[ \t]+(\S+)[ \t\r]*$`)
	textPkgFail  = regexp.MustCompile(`^(FAIL|panic:)`)
)

// ParseText parses a single-package `go test -v` log, attributing it to pkg.
func ParseText(r io.Reader, pkg string) (*Log, error) {
	l := newLog()
	n := 0
	err := readLines(r, func(line string) error {
		n++
		if m := textRunRe.FindStringSubmatch(line); m != nil {
			l.obs(pkg, m[1]).Ran = true
			return nil
		}
		if m := textResultRe.FindStringSubmatch(line); m != nil {
			o := l.obs(pkg, m[2])
			sub := strings.Contains(m[2], "/")
			o.Ran = true
			switch m[1] {
			case "PASS":
				if !sub {
					o.Passed = true
				}
			case "SKIP":
				o.Skipped = append(o.Skipped, m[2])
			case "FAIL":
				if sub {
					o.FailedSub = append(o.FailedSub, m[2])
				} else {
					o.Failed = true
				}
			}
			return nil
		}
		if textPkgFail.MatchString(line) {
			l.FailedPackages[pkg] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errors.New("empty log")
	}
	return l, nil
}

type Row struct {
	ID      string `json:"id"`
	Stage   string `json:"stage"`
	Package string `json:"package"`
	Test    string `json:"test"`
	Status  string `json:"status"`
	Detail  string `json:"detail,omitempty"`
}

type Report struct {
	Gate   string `json:"gate"`
	SHA    string `json:"sha"`
	Pass   bool   `json:"pass"`
	Passed int    `json:"passed"`
	Total  int    `json:"total"`
	Rows   []Row  `json:"rows"`
}

// Evaluate decides every manifest entry. Pass is true only if all are PASS.
func Evaluate(m *Manifest, l *Log, sha string) *Report {
	rep := &Report{Gate: m.Name, SHA: sha, Total: len(m.Entries)}
	for _, e := range m.Entries {
		row := Row{ID: e.ID, Stage: e.Stage, Package: e.Package, Test: e.Test}
		o := l.Tests[key(e.Package, e.Test)]
		switch {
		case o == nil:
			row.Status, row.Detail = StatusMissing, "test did not run (absent, package did not build, or no tests to run)"
		case len(o.Skipped) > 0:
			row.Status, row.Detail = StatusSkip, "skipped: "+strings.Join(o.Skipped, ",")
		case o.Failed || len(o.FailedSub) > 0:
			row.Status, row.Detail = StatusFail, "failed: "+strings.Join(append([]string{e.Test}, o.FailedSub...), ",")
		case !o.Passed:
			row.Status, row.Detail = StatusRunOnly, "no top-level PASS result (RUN-only, panic, timeout or truncation)"
		case l.FailedPackages[e.Package]:
			row.Status, row.Detail = StatusFail, "package reported FAIL"
		default:
			row.Status = StatusPass
			rep.Passed++
		}
		rep.Rows = append(rep.Rows, row)
	}
	sort.SliceStable(rep.Rows, func(i, j int) bool { return rep.Rows[i].ID < rep.Rows[j].ID })
	rep.Pass = rep.Passed == rep.Total && rep.Total > 0
	return rep
}

// CheckSHA requires the exact 40-hex commit under test to equal the expected one.
func CheckSHA(head, expected string) error {
	if !shaRe.MatchString(head) || !shaRe.MatchString(expected) {
		return errors.New("sha: both the checked-out HEAD and the expected SHA must be 40 lowercase hex")
	}
	if head != expected {
		return fmt.Errorf("sha: checked-out %s is not the exact commit under test %s", head, expected)
	}
	return nil
}

func run(args []string, stdout, stderr io.Writer) int {
	var manifestPath, logPath, format, pkg, head, expect, out string
	for i := 0; i < len(args); i++ {
		next := func() string {
			i++
			if i >= len(args) {
				fmt.Fprintf(stderr, "opgate: missing value for %s\n", args[i-1])
				return ""
			}
			return args[i]
		}
		switch args[i] {
		case "-manifest":
			manifestPath = next()
		case "-log":
			logPath = next()
		case "-format":
			format = next()
		case "-package":
			pkg = next()
		case "-head-sha":
			head = next()
		case "-expect-sha":
			expect = next()
		case "-out":
			out = next()
		default:
			fmt.Fprintf(stderr, "opgate: unknown argument %q\n", args[i])
			return 2
		}
	}
	if manifestPath == "" || logPath == "" || head == "" || expect == "" {
		fmt.Fprintln(stderr, "usage: opgate -manifest M -log L -head-sha SHA -expect-sha SHA [-format json|text] [-package ./pkg] [-out report.json]")
		return 2
	}
	if err := CheckSHA(head, expect); err != nil {
		fmt.Fprintln(stderr, "opgate: "+err.Error())
		return 1
	}
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		fmt.Fprintln(stderr, "opgate:", err)
		return 2
	}
	m, err := LoadManifest(raw)
	if err != nil {
		fmt.Fprintln(stderr, "opgate:", err)
		return 2
	}
	f, err := os.Open(logPath)
	if err != nil {
		fmt.Fprintln(stderr, "opgate:", err)
		return 1
	}
	defer f.Close()
	var l *Log
	switch format {
	case "", "json":
		l, err = ParseJSON(f)
	case "text":
		if pkg == "" {
			fmt.Fprintln(stderr, "opgate: -format text needs -package")
			return 2
		}
		l, err = ParseText(f, pkg)
	default:
		fmt.Fprintln(stderr, "opgate: unknown format")
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, "opgate: unreadable log (fail closed):", err)
		return 1
	}
	rep := Evaluate(m, l, expect)
	for _, r := range rep.Rows {
		fmt.Fprintf(stdout, "%-8s %-24s %s %s %s\n", r.Status, r.Stage, r.Package, r.Test, r.Detail)
	}
	if out != "" {
		b, _ := json.MarshalIndent(rep, "", "  ")
		if err := os.WriteFile(out, append(b, '\n'), 0o644); err != nil {
			fmt.Fprintln(stderr, "opgate:", err)
			return 1
		}
	}
	if !rep.Pass {
		fmt.Fprintf(stdout, "GATE RED: %d/%d required operational ingress v1 tests passed (zero SKIP required)\n", rep.Passed, rep.Total)
		return 1
	}
	fmt.Fprintf(stdout, "GATE GREEN: %d/%d @ %s\n", rep.Passed, rep.Total, expect)
	return 0
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
