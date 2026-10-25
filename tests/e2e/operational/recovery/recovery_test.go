//go:build linux

package recovery

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// Real-kernel restart-recovery tests. Each test owns a private network
// namespace (created and deleted here, the same precedent as run_lab.sh) and a
// durable state directory on the private tmpfs. The Agent incarnations are
// SEPARATE processes (recoverylab, started through `ip netns exec`) so that
// a SIGKILL really closes their sockets in the kernel; only one of them lives
// at a time because the product's inventory requires a single process in the
// lab namespace. The orchestrator never runs a helper inside the namespace
// while an incarnation is working.

var seq atomic.Int32

type lab struct {
	t    *testing.T
	ns   string
	dir  string
	port int
}

const siblingTag = "baft-ingress-00000000000000000000000000000001"

func newLab(t *testing.T, port int) *lab {
	t.Helper()
	if !enabled() {
		t.Skip("restart-recovery LAB disabled (set " + gateEnv + "=1; see .github/workflows/operational-recovery.yml)")
	}
	l := &lab{t: t, ns: fmt.Sprintf("oprec-%d-%d", os.Getpid(), seq.Add(1)), port: port}
	l.dir = filepath.Join("/dev/shm", l.ns)
	if err := os.MkdirAll(l.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("ip", "netns", "add", l.ns).CombinedOutput(); err != nil {
		t.Fatalf("netns add: %v %s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("ip", "netns", "del", l.ns).Run()
		_ = os.RemoveAll(l.dir)
	})
	l.sh("ip", "link", "set", "lo", "up")
	l.sh("nft", "add", "table", "inet", "baft")
	l.sh("nft", "add", "chain", "inet", "baft", "ingress", "{ type filter hook input priority -100; policy accept; }")
	// A sibling tunnel's rule and an unrelated foreign table: both must survive.
	l.sh("nft", "add", "rule", "inet", "baft", "ingress", "tcp", "dport", "40001", "accept", "comment", siblingTag)
	l.sh("nft", "add", "table", "inet", "op_keep")
	l.sh("nft", "add", "chain", "inet", "op_keep", "keep")
	l.sh("nft", "add", "rule", "inet", "op_keep", "keep", "tcp", "dport", "2053", "accept")
	// The installed policy pins this namespace's identity; earlier tests'
	// copy (on the private tmpfs) must not leak into this namespace.
	if err := os.RemoveAll("/etc/baft/ingress"); err != nil {
		t.Fatal(err)
	}
	if r := l.run("init"); r["ok"] != true {
		t.Fatalf("init: %v", r)
	}
	return l
}

func (l *lab) nsCmd(name string, args ...string) *exec.Cmd {
	return exec.Command("ip", append([]string{"netns", "exec", l.ns, name}, args...)...)
}

func (l *lab) sh(name string, args ...string) string {
	l.t.Helper()
	out, err := l.nsCmd(name, args...).CombinedOutput()
	if err != nil {
		l.t.Fatalf("%s %v: %v %s", name, args, err, out)
	}
	return string(out)
}

func (l *lab) labArgs(extra []string, phase string) []string {
	a := []string{"-dir", l.dir, "-port", fmt.Sprint(l.port)}
	return append(append(a, extra...), phase)
}

// run executes one incarnation to completion and returns its RESULT object.
func (l *lab) run(phase string, extra ...string) map[string]any {
	l.t.Helper()
	cmd := l.nsCmd(labBin, l.labArgs(extra, phase)...)
	cmd.Env = append(os.Environ(), "SPENT="+os.Getenv("SPENT"))
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		l.t.Fatalf("lab %s: %v\nstdout: %s\nstderr: %s", phase, err, out.String(), errb.String())
	}
	return parseResult(l.t, out.String())
}

func parseResult(t *testing.T, out string) map[string]any {
	t.Helper()
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		if rest, ok := strings.CutPrefix(sc.Text(), "RESULT "); ok {
			var m map[string]any
			if err := json.Unmarshal([]byte(rest), &m); err != nil {
				t.Fatalf("bad RESULT %q: %v", rest, err)
			}
			return m
		}
	}
	t.Fatalf("no RESULT line in: %s", out)
	return nil
}

// runKilled starts an incarnation that blocks at fault, waits for its marker,
// then kill -9s it. The nft rule set and every durable directory stay.
func (l *lab) runKilled(phase, fault string) {
	l.t.Helper()
	cmd := l.nsCmd(labBin, l.labArgs([]string{"-fault", fault}, phase)...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Start(); err != nil {
		l.t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	marker := filepath.Join(l.dir, "at-"+fault)
	deadline := time.After(90 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		select {
		case err := <-done:
			l.t.Fatalf("incarnation ended before reaching %s: %v\nstdout: %s\nstderr: %s", fault, err, out.String(), errb.String())
		case <-deadline:
			_ = cmd.Process.Kill()
			l.t.Fatalf("timeout waiting for %s\nstdout: %s\nstderr: %s", fault, out.String(), errb.String())
		case <-time.After(50 * time.Millisecond):
		}
	}
	if err := cmd.Process.Signal(syscall.SIGKILL); err != nil {
		l.t.Fatal(err)
	}
	err := <-done
	if ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); !ok || !ws.Signaled() || ws.Signal() != syscall.SIGKILL {
		l.t.Fatalf("incarnation was not killed by SIGKILL: %v", err)
	}
}

func (l *lab) chain() string { return l.sh("nft", "-j", "list", "chain", "inet", "baft", "ingress") }
func (l *lab) keep() string  { return l.sh("nft", "-j", "list", "table", "inet", "op_keep") }
func (l *lab) rules() int    { return strings.Count(l.chain(), "baft-ingress-") }

func (l *lab) phase() string {
	p := l.run("probe")
	s, _ := p["phase"].(string)
	return s
}

func (l *lab) reconcile(extra ...string) (outcome, reason, direction string, raw map[string]any) {
	l.t.Helper()
	raw = l.run("reconcile", extra...)
	rs, _ := raw["results"].([]any)
	if len(rs) != 1 {
		l.t.Fatalf("reconcile results: %v", raw)
	}
	r := rs[0].(map[string]any)
	return fmt.Sprint(r["Outcome"]), fmt.Sprint(r["Reason"]), fmt.Sprint(r["Direction"]), raw
}

// expectRecovered asserts the post-recovery host state: the owner rule is gone,
// the sibling rule and the foreign table are byte-identical, the journal is
// retired and the reservation stays fenced.
func (l *lab) expectRecovered(keepBefore string) {
	l.t.Helper()
	if n := l.rules(); n != 1 || !strings.Contains(l.chain(), siblingTag) {
		l.t.Fatalf("rule set after recovery wrong (owned=%d): %s", n, l.chain())
	}
	if l.keep() != keepBefore {
		l.t.Fatal("foreign nft table changed")
	}
	p := l.run("probe")
	if p["phase"] != "retired" {
		l.t.Fatalf("journal after recovery: %v", p)
	}
	if st, _ := p["states"].([]any); len(st) != 1 || st[0] != "NEEDS_RECONCILIATION" {
		l.t.Fatalf("reservation not left fenced for the operator: %v", p)
	}
}

func TestOperationalRecovery_KillMidApplyRuleInstalledBeforeVerified(t *testing.T) {
	l := newLab(t, 54811)
	keep := l.keep()
	l.runKilled("apply", "before:verified")
	if l.phase() != "firewall_installed" || l.rules() != 2 {
		t.Fatalf("fixture: journal %s rules %d", l.phase(), l.rules())
	}
	chain := l.chain()
	// Report mode evaluates every evidence rule and writes nothing.
	if out, _, _, raw := l.reconcile("-mode", "report"); out != "would_reconcile" || l.chain() != chain || l.phase() != "firewall_installed" {
		t.Fatalf("report mode: %s %v", out, raw)
	}
	out, reason, dir, raw := l.reconcile()
	t.Logf("recovery: %s (%s): %s\n%v", out, dir, reason, raw)
	if out != "rolled_back" || dir != "apply" {
		t.Fatalf("outcome %s/%s: %s", out, dir, reason)
	}
	l.expectRecovered(keep)
	if out, _, _, _ := l.reconcile(); out != "already_retired" {
		t.Fatalf("second run not idempotent: %s", out)
	}
}

func TestOperationalRecovery_KillMidApplyRuleInstalledButNotJournaled(t *testing.T) {
	l := newLab(t, 54812)
	keep := l.keep()
	l.runKilled("apply", "after:add")
	if l.phase() != "listener_bound" || l.rules() != 2 {
		t.Fatalf("fixture: journal %s rules %d", l.phase(), l.rules())
	}
	if out, reason, _, raw := l.reconcile(); out != "rolled_back" {
		t.Fatalf("outcome %s: %s %v", out, reason, raw)
	}
	l.expectRecovered(keep)
}

func TestOperationalRecovery_KillMidRetireBeforeRuleDelete(t *testing.T) {
	l := newLab(t, 54813)
	keep := l.keep()
	l.runKilled("apply-retire", "before:delete")
	if l.phase() != "retire_intent" || l.rules() != 2 {
		t.Fatalf("fixture: journal %s rules %d", l.phase(), l.rules())
	}
	out, reason, dir, raw := l.reconcile()
	t.Logf("recovery: %s (%s): %s\n%v", out, dir, reason, raw)
	if out != "retired" || dir != "retire" {
		t.Fatalf("outcome %s/%s: %s", out, dir, reason)
	}
	l.expectRecovered(keep)
}

func TestOperationalRecovery_KillMidRetireAfterIntentWithListenerStillLive(t *testing.T) {
	l := newLab(t, 54814)
	keep := l.keep()
	l.runKilled("apply-retire", "after:retire_intent")
	if l.phase() != "retire_intent" || l.rules() != 2 {
		t.Fatalf("fixture: journal %s rules %d", l.phase(), l.rules())
	}
	if out, reason, _, raw := l.reconcile(); out != "retired" {
		t.Fatalf("outcome %s: %s %v", out, reason, raw)
	}
	l.expectRecovered(keep)
}

// The ordinary restart: a verified, serving ingress whose Agent process simply
// exits. The listener died with it; the dead accept rule is removed.
func TestOperationalRecovery_VerifiedIngressAfterPlainAgentRestart(t *testing.T) {
	l := newLab(t, 54815)
	keep := l.keep()
	if r := l.run("apply"); r["err"] != "" {
		t.Fatalf("apply: %v", r)
	}
	if l.phase() != "verified" || l.rules() != 2 {
		t.Fatalf("fixture: journal %s rules %d", l.phase(), l.rules())
	}
	if out, reason, dir, _ := l.reconcile(); out != "rolled_back" || dir != "apply" {
		t.Fatalf("outcome %s/%s: %s", out, dir, reason)
	}
	l.expectRecovered(keep)
}

// A listener on the port that is not ours is reported and never touched, in
// either family, and nothing else is changed; once it is gone the same
// evidence completes the recovery.
func TestOperationalRecovery_ForeignListenerFencesAndIsNeverTouched(t *testing.T) {
	for _, family := range []string{"tcp4", "tcp6"} {
		t.Run(family, func(t *testing.T) {
			l := newLab(t, 54816)
			keep := l.keep()
			l.runKilled("apply-retire", "before:delete")
			chain := l.chain()
			out, reason, _, raw := l.reconcile("-foreign", family)
			t.Logf("%s: %s: %s\n%v", out, family, reason, raw)
			if out != "fenced" || !strings.Contains(reason, "listener") {
				t.Fatalf("foreign listener not fenced with evidence: %s %s", out, reason)
			}
			socks := raw["results"].([]any)[0].(map[string]any)["Report"].(map[string]any)["Sockets"].([]any)
			if len(socks) == 0 || fmt.Sprint(socks[0].(map[string]any)["Class"]) != "foreign-listener" {
				t.Fatalf("socket evidence missing: %v", socks)
			}
			if l.chain() != chain || l.phase() != "retire_intent" {
				t.Fatal("a fenced decision changed the rule set or the journal")
			}
			if out, _, _, _ := l.reconcile(); out != "retired" {
				t.Fatalf("recovery did not complete once the port was free: %s", out)
			}
			l.expectRecovered(keep)
		})
	}
}

// An admitted-but-never-leased command and a rule nobody journaled authorize
// nothing: recovery fences and leaves the rule alone.
func TestOperationalRecovery_UnleasedIntentAndUnjournaledRuleAreNeverTouched(t *testing.T) {
	l := newLab(t, 54817)
	if r := l.run("plant"); r["err"] != "" {
		t.Fatalf("plant: %v", r)
	}
	if r := l.run("intent-only"); r["err"] != "" {
		t.Fatalf("intent-only: %v", r)
	}
	chain := l.chain()
	out, reason, _, _ := l.reconcile()
	if out != "fenced" || !strings.Contains(reason, "no restart authority") {
		t.Fatalf("outcome %s: %s", out, reason)
	}
	if l.chain() != chain {
		t.Fatal("rule set changed without a consumed lease")
	}
}

// Regression for the Retire ping-pong class (security review #1): recovery
// never revives a spent job. Both consumed leases stay consumed in every later
// incarnation, and a NEW signed Retire after recovery is inert.
func TestOperationalRecovery_RecoveryCannotReviveSpentJobs(t *testing.T) {
	l := newLab(t, 54818)
	keep := l.keep()
	l.runKilled("apply-retire", "before:delete")
	check := func(when string) {
		for _, which := range []string{"apply", "retire"} {
			t.Setenv("SPENT", which)
			r := l.run("mint-spent")
			// Before recovery the durable single-use fence answers; after it the
			// operation is NEEDS_RECONCILIATION, where an Apply lease is refused
			// by state (and a Retire by its fence). Never minted either way.
			replay := strings.Contains(fmt.Sprint(r["err"]), "already minted")
			if r["minted"] == true || r["err"] == "" || (when == "before recovery" && !replay) {
				t.Fatalf("%s: spent %s job minted again: %v", when, which, r)
			}
		}
	}
	check("before recovery")
	if out, _, _, _ := l.reconcile(); out != "retired" {
		t.Fatalf("outcome %s", out)
	}
	check("after recovery")
	before := l.chain()
	r := l.run("retire-new")
	if r["err"] == "" {
		t.Fatalf("a new Retire after recovery claimed success: %v", r)
	}
	if l.chain() != before || l.phase() != "retired" {
		t.Fatal("a new Retire after recovery changed the host or the journal")
	}
	l.expectRecoveredAllowingIntents(keep)
}

// expectRecoveredAllowingIntents is expectRecovered for runs where a later
// signed job was recorded (the reservation state stays one of the two fenced
// states and the host stays clean).
func (l *lab) expectRecoveredAllowingIntents(keepBefore string) {
	l.t.Helper()
	if n := l.rules(); n != 1 || !strings.Contains(l.chain(), siblingTag) || l.keep() != keepBefore {
		l.t.Fatalf("host not clean after recovery: %s", l.chain())
	}
	if l.phase() != "retired" {
		l.t.Fatalf("journal %s", l.phase())
	}
}
