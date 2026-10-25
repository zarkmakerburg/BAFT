package release_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These tests drive the real baft-pair binary through the standalone
// (no-BCC) pairing handshake and its failure modes. They do not prove
// two-host reachability, SSH blocking or systemd.

type pairRig struct {
	t    *testing.T
	bin  string
	work string
}

func newPairRig(t *testing.T) *pairRig {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	bin := filepath.Join(work, "baft-pair")
	build := exec.Command("go", "build", "-o", bin, "./cmd/baft-pair")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build baft-pair: %v: %s", err, out)
	}
	r := &pairRig{t: t, bin: bin, work: work}
	if out, err := r.run("pki", "--dir", filepath.Join(work, "pki"), "--host", "127.0.0.1"); err != nil {
		t.Fatalf("pki: %v %s", err, out)
	}
	return r
}

func (r *pairRig) run(args ...string) (string, error) {
	out, err := exec.Command(r.bin, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (r *pairRig) p(name string) string { return filepath.Join(r.work, name) }

// exCode creates an EX pairing code and its pending state.
func (r *pairRig) exCode(tag, ttl string) string {
	r.t.Helper()
	out, err := r.run("ex-code", "--key", r.p("ex-"+tag+".key"), "--address", "127.0.0.1:18443",
		"--server-name", "127.0.0.1", "--identity", "urn:baft:node:ex-"+tag, "--ca-file", r.p("pki/ca.pem"),
		"--psk-out", r.p(tag+".psk"), "--pending-out", r.p(tag+".pending"), "--ttl", ttl)
	if err != nil {
		r.t.Fatalf("ex-code: %v %s", err, out)
	}
	return out
}

func (r *pairRig) irApply(tag, code string) (string, error) {
	return r.run("ir-apply", "--code", code, "--key", r.p("ir-"+tag+".key"), "--state-dir", r.p("irstate-"+tag),
		"--config-out", r.p("ir-"+tag+".yaml"), "--identity", "urn:baft:node:ir-"+tag)
}

func (r *pairRig) exAccept(tag, exTag, reply string) (string, error) {
	return r.run("ex-accept", "--reply", reply, "--pending", r.p(exTag+".pending"), "--psk-file", r.p(exTag+".psk"),
		"--key", r.p("ex-"+exTag+".key"), "--ca-file", r.p("pki/ca.pem"), "--cert-file", r.p("pki/server.pem"),
		"--cert-key-file", r.p("pki/server.key"), "--config-out", r.p("ex-"+tag+".yaml"), "--unix-socket", r.p("admin.sock"))
}

func exists(path string) bool { _, err := os.Lstat(path); return err == nil }

func TestGammaStandalonePairingRejectsMalformedAndTamperedCodes(t *testing.T) {
	r := newPairRig(t)
	good := r.exCode("m", "5m")
	mid := len(good) / 2
	flip := "A"
	if good[mid:mid+1] == "A" {
		flip = "B"
	}
	for name, code := range map[string]string{
		"truncated":     good[:len(good)-12],
		"tampered":      good[:mid] + flip + good[mid+1:],
		"empty":         "",
		"prefix-only":   "BAFTPAIR1:",
		"reply-as-code": "BAFTREPLY1:AAAA",
		"whitespace":    " \n ",
	} {
		t.Run(name, func(t *testing.T) {
			tag := strings.ReplaceAll(name, "-", "")
			out, err := r.irApply(tag, code)
			if err == nil {
				t.Fatalf("malformed code accepted: %s", out)
			}
			for _, f := range []string{"ir-" + tag + ".key", "irstate-" + tag, "ir-" + tag + ".yaml"} {
				if exists(r.p(f)) {
					t.Fatalf("rejected code created %s", f)
				}
			}
		})
	}
	// The untouched code still pairs: rejections did not burn it.
	if out, err := r.irApply("ok", good); err != nil || !strings.HasPrefix(out, "BAFTREPLY1:") {
		t.Fatalf("valid code after rejections: %v %s", err, out)
	}
}

func TestGammaStandalonePairingRejectsBadRepliesAndKeepsPendingForRetry(t *testing.T) {
	r := newPairRig(t)
	code := r.exCode("a", "5m")
	reply, err := r.irApply("a", code)
	if err != nil {
		t.Fatalf("ir-apply: %v %s", err, reply)
	}
	other := r.exCode("b", "5m")
	otherReply, err := r.irApply("b", other)
	if err != nil {
		t.Fatalf("ir-apply b: %v %s", err, otherReply)
	}
	mid := len(reply) / 2
	flip := "A"
	if reply[mid:mid+1] == "A" {
		flip = "B"
	}
	bad := map[string]string{
		"truncated":     reply[:len(reply)-10],
		"tampered":      reply[:mid] + flip + reply[mid+1:],
		"other-pairing": otherReply,
		"wrong-prefix":  "BAFTPAIR1:" + strings.TrimPrefix(reply, "BAFTREPLY1:"),
		"empty-payload": "BAFTREPLY1:",
	}
	for name, rep := range bad {
		out, err := r.exAccept("rej-"+name, "a", rep)
		if err == nil {
			t.Fatalf("%s reply accepted: %s", name, out)
		}
		if exists(r.p("ex-rej-" + name + ".yaml")) {
			t.Fatalf("%s reply produced a config", name)
		}
		if !exists(r.p("a.pending")) || !exists(r.p("a.psk")) {
			t.Fatalf("%s reply consumed the pending pairing", name)
		}
	}
	// The right reply still completes, once.
	if out, err := r.exAccept("done", "a", reply); err != nil {
		t.Fatalf("valid reply after rejections: %v %s", err, out)
	}
	if !exists(r.p("ex-done.yaml")) {
		t.Fatal("no EX config after valid reply")
	}
	if exists(r.p("a.pending")) || exists(r.p("a.psk")) {
		t.Fatal("one-time pairing secrets survived acceptance")
	}
	// Replay of the same reply must fail and must not rewrite the config.
	before, _ := os.ReadFile(r.p("ex-done.yaml"))
	if out, err := r.exAccept("replay", "a", reply); err == nil {
		t.Fatalf("replayed reply accepted: %s", out)
	}
	if exists(r.p("ex-replay.yaml")) {
		t.Fatal("replay produced a config")
	}
	after, _ := os.ReadFile(r.p("ex-done.yaml"))
	if string(before) != string(after) {
		t.Fatal("replay altered the accepted config")
	}
}

func TestGammaStandalonePairingExpiredReplyIsRejected(t *testing.T) {
	r := newPairRig(t)
	code := r.exCode("e", "2s")
	reply, err := r.irApply("e", code)
	if err != nil {
		t.Fatalf("ir-apply before expiry: %v %s", err, reply)
	}
	time.Sleep(3200 * time.Millisecond)
	out, err := r.exAccept("late", "e", reply)
	if err == nil {
		t.Fatalf("expired reply accepted: %s", out)
	}
	if !strings.Contains(strings.ToLower(out), "expired") && !strings.Contains(strings.ToLower(out), "invalid") {
		t.Fatalf("unexpected rejection text: %s", out)
	}
	if exists(r.p("ex-late.yaml")) {
		t.Fatal("expired reply produced a config")
	}
}
