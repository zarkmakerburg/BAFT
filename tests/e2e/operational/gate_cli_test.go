//go:build linux

package operational

import (
	"bytes"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/bcc"
)

// binary returns the compiled CLI: $env if set (prebuilt LAB image), else it
// is built from this checkout with the Go toolchain.
func binary(t *testing.T, env, pkg string) string {
	t.Helper()
	if p := os.Getenv(env); p != "" {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("%s=%s: %v", env, p, err)
		}
		return p
	}
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "..")
	out := filepath.Join(t.TempDir(), filepath.Base(pkg))
	cmd := exec.Command("go", "build", "-o", out, pkg)
	cmd.Dir = root
	if raw, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v: %s", pkg, err, raw)
	}
	return out
}

func run(t *testing.T, bin string, args ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	return strings.TrimSpace(so.String()), se.String(), err
}

// restart simulates a BCC outage and recovery on the SAME address: the HTTPS
// server stops (the port becomes inaccessible) and a new process reopens the
// persisted store behind the same identity.
func (b *bccRig) stop() string {
	addr := b.srv.Listener.Addr().String()
	b.srv.Close()
	return addr
}

func (b *bccRig) startAt(addr string) {
	b.t.Helper()
	store, err := bcc.OpenStore(b.store.Path())
	if err != nil {
		b.t.Fatal(err)
	}
	app, err := bcc.NewServer(store, adminTk)
	if err != nil {
		b.t.Fatal(err)
	}
	app.ConfigureJobSigning(b.key)
	var ln net.Listener
	for i := 0; i < 50; i++ {
		if ln, err = net.Listen("tcp", addr); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		b.t.Fatalf("rebind BCC address: %v", err)
	}
	srv := b.pki.serverOn(b.t, app.Handler(), ln)
	b.store, b.app, b.srv = store, app, srv
}

// Pairing is a standalone CLI protocol: it needs no BCC. A failed or
// forged pairing never damages existing state and can be repaired by
// pairing again; and an applied public ingress keeps serving through a BCC
// outage, after which the Agent and BCC simply resume.
func TestOperationalIngressV1Gate_CLIPairingWithInaccessibleBCCAndRepair(t *testing.T) {
	pair := binary(t, "BAFT_PAIR_BIN", "./cmd/baft-pair")
	baft := binary(t, "BAFT_BIN", "./cmd/baft")
	// An address nothing listens on stands in for the inaccessible BCC; the
	// pairing CLIs must never need it.
	dead, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("loopback networking is required: %v", err)
	}
	deadAddr := dead.Addr().String()
	dead.Close()
	t.Setenv("BAFT_BCC_URL", "https://"+deadAddr)
	t.Setenv("HTTPS_PROXY", "http://"+deadAddr)

	work := t.TempDir()
	ex, ir := filepath.Join(work, "ex"), filepath.Join(work, "ir")
	for _, d := range []string{ex, filepath.Join(ir, "state")} {
		if err := os.MkdirAll(d, 0700); err != nil {
			t.Fatal(err)
		}
	}
	mustRun := func(bin string, args ...string) string {
		t.Helper()
		so, se, err := run(t, bin, args...)
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", filepath.Base(bin), args, err, se)
		}
		return so
	}
	mustRun(pair, "keygen", "--file", filepath.Join(ex, "noise-key.json"))
	mustRun(pair, "keygen", "--file", filepath.Join(ir, "noise-key.json"))
	mustRun(pair, "pki", "--dir", filepath.Join(ex, "pki"), "--host", "127.0.0.1")
	exCode := func(tag, ttl string) string {
		return mustRun(pair, "ex-code", "--key", filepath.Join(ex, "noise-key.json"), "--address", "127.0.0.1:14443",
			"--server-name", "127.0.0.1", "--identity", "urn:baft:node:ex-"+tag, "--ca-file", filepath.Join(ex, "pki", "ca.pem"),
			"--psk-out", filepath.Join(ex, "pairing.psk"), "--pending-out", filepath.Join(ex, "pairing.ex.json"), "--ttl", ttl)
	}
	irApply := func(code string, extra ...string) (string, string, error) {
		args := append([]string{"ir-apply", "--code", code, "--key", filepath.Join(ir, "noise-key.json"), "--state-dir", filepath.Join(ir, "state"),
			"--config-out", filepath.Join(ir, "baft.yaml"), "--route-listen", "127.0.0.1:14444", "--metrics-listen", "127.0.0.1:14445"}, extra...)
		return run(t, pair, args...)
	}
	exAccept := func(reply string) (string, error) {
		_, se, err := run(t, pair, "ex-accept", "--reply", reply, "--pending", filepath.Join(ex, "pairing.ex.json"), "--psk-file", filepath.Join(ex, "pairing.psk"),
			"--key", filepath.Join(ex, "noise-key.json"), "--listen", "127.0.0.1:14443", "--ca-file", filepath.Join(ex, "pki", "ca.pem"),
			"--cert-file", filepath.Join(ex, "pki", "server.pem"), "--cert-key-file", filepath.Join(ex, "pki", "server.key"),
			"--target", "127.0.0.1:14446", "--metrics-listen", "127.0.0.1:14447", "--unix-socket", filepath.Join(ex, "admin.sock"),
			"--config-out", filepath.Join(ex, "baft.yaml"))
		return se, err
	}

	// 1. A rejected / expired code leaves no IR state at all.
	short := exCode("short", "1s")
	time.Sleep(1300 * time.Millisecond)
	if so, se, err := irApply(short); err == nil || !strings.Contains(strings.ToLower(se+so), "expired") {
		t.Fatalf("expired pairing code accepted: %v %s %s", err, so, se)
	}
	if _, err := os.Lstat(filepath.Join(ir, "baft.yaml")); !os.IsNotExist(err) {
		t.Fatal("a rejected code wrote IR config")
	}
	if so, se, err := irApply("BAFTPAIR0:not-a-code"); err == nil {
		t.Fatalf("malformed code accepted: %s %s", so, se)
	}

	// 2. Pair for real, with a forged reply tried first: it is refused and
	//    the pending pairing survives so the genuine reply still works.
	code := exCode("main", "5m")
	reply, se, err := irApply(code)
	if err != nil || !strings.HasPrefix(reply, "BAFTREPLY1:") {
		t.Fatalf("ir-apply: %v %q %s", err, reply, se)
	}
	rogueDir := filepath.Join(work, "rogue")
	_ = os.MkdirAll(filepath.Join(rogueDir, "state"), 0700)
	mustRun(pair, "keygen", "--file", filepath.Join(rogueDir, "noise-key.json"))
	rogueCode := mustRun(pair, "ex-code", "--key", filepath.Join(rogueDir, "noise-key.json"), "--address", "127.0.0.1:1", "--server-name", "x",
		"--identity", "urn:baft:node:rogue", "--ca-file", filepath.Join(ex, "pki", "ca.pem"), "--psk-out", filepath.Join(rogueDir, "p.psk"), "--ttl", "5m")
	mustRun(pair, "keygen", "--file", filepath.Join(rogueDir, "ir.json"))
	rogueReply := mustRun(pair, "ir-apply", "--code", rogueCode, "--key", filepath.Join(rogueDir, "ir.json"), "--state-dir", filepath.Join(rogueDir, "state"),
		"--config-out", filepath.Join(rogueDir, "baft.yaml"))
	if se, err := exAccept(rogueReply); err == nil || !strings.Contains(se, "not authentic") {
		t.Fatalf("forged reply accepted: %v %s", err, se)
	}
	if _, err := os.Stat(filepath.Join(ex, "baft.yaml")); !os.IsNotExist(err) {
		t.Fatal("a forged reply produced EX config")
	}
	if _, err := os.Stat(filepath.Join(ex, "pairing.ex.json")); err != nil {
		t.Fatal("a forged reply consumed the pending pairing")
	}
	if se, err := exAccept(reply); err != nil {
		t.Fatalf("genuine reply after the forged one: %v %s", err, se)
	}
	for _, f := range []string{"pairing.psk", "pairing.ex.json"} {
		if _, err := os.Stat(filepath.Join(ex, f)); !os.IsNotExist(err) {
			t.Fatalf("one-time pairing secret %s not removed", f)
		}
	}
	// A consumed pairing cannot be accepted twice.
	if _, err := exAccept(reply); err == nil {
		t.Fatal("the same reply was accepted twice")
	}
	for _, cfg := range []string{filepath.Join(ex, "baft.yaml"), filepath.Join(ir, "baft.yaml")} {
		mustRun(baft, "config", "validate", "--file", cfg)
	}
	irBefore, _ := os.ReadFile(filepath.Join(ir, "baft.yaml"))

	// 3. Repair: a fresh pairing over the same directories either replaces
	//    the IR config with a valid one or refuses and leaves the old one
	//    byte-identical; it must never leave a half-written config.
	code2 := exCode("again", "5m")
	reply2, se, err := irApply(code2)
	irAfter, _ := os.ReadFile(filepath.Join(ir, "baft.yaml"))
	if err != nil {
		if !bytes.Equal(irBefore, irAfter) {
			t.Fatalf("a refused re-pair modified the working IR config (%s)", se)
		}
	} else {
		mustRun(baft, "config", "validate", "--file", filepath.Join(ir, "baft.yaml"))
		if se, err := exAccept(reply2); err != nil {
			t.Fatalf("EX could not accept the repaired pairing: %v %s", err, se)
		}
		mustRun(baft, "config", "validate", "--file", filepath.Join(ex, "baft.yaml"))
	}

	// 4. BCC outage and recovery around a live operational ingress.
	f := setupFullPath(t, 54811, "127.0.0.1:1443")
	f.legit(t)
	rules := f.h.chainJSON()
	addr := f.b.stop()
	if _, err := f.a.Agent.RunOnce(f.a.proc); err == nil {
		t.Fatal("the Agent reported a poll success with BCC inaccessible")
	}
	if err := roundTrip(t, f.addr(), []byte("through-the-outage")); err != nil {
		t.Fatalf("an applied ingress stopped serving when BCC became inaccessible: %v", err)
	}
	if f.h.chainJSON() != rules {
		t.Fatal("the Agent changed host state during the BCC outage")
	}
	if left, err := f.a.Store.InspectFencedLeftovers(); err != nil || len(left) != 1 {
		t.Fatalf("ownership state changed during the outage: %+v %v", left, err)
	}
	f.b.startAt(addr)
	f.a.openGate()
	if v := f.waitReady(t); !v.Ready || v.Card == nil {
		t.Fatalf("the verified card did not come back after the BCC returned: %+v", v)
	}
	f.enqueueRetire(t)
	if res := f.a.runOnce(); len(res) != 1 || res[0].Status != "succeeded" {
		t.Fatalf("a signed Retire after the outage: %+v", res)
	}
	waitPortFree(t, f.port)
}
