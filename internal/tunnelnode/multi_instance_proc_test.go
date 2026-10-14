package tunnelnode

import (
	"bytes"
	"context"
	"errors"
	"github.com/zarkmakerburg/baft/internal/verification"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// A host can run several real BAFT processes with one filesystem namespace.
// Only systemd is simulated; pairing, TLS/Noise, payload and services are real.
type instanceProcessHost struct {
	mu          sync.Mutex
	hosts       map[string]*procHost
	failRestart string
	logDir      string
}

func newInstanceProcessHost(t *testing.T) *instanceProcessHost {
	t.Helper()
	h := &instanceProcessHost{hosts: map[string]*procHost{}, logDir: t.TempDir()}
	t.Cleanup(func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		for name, p := range h.hosts {
			p.mu.Lock()
			p.stopLocked()
			p.mu.Unlock()
			if t.Failed() {
				b, _ := os.ReadFile(p.log)
				if len(b) > 6000 {
					b = b[len(b)-6000:]
				}
				t.Logf("fixture %s runtime log:\n%s", name, b)
			}
		}
	})
	return h
}

func (h *instanceProcessHost) register(m *Manager) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hosts[m.Service] = &procHost{config: m.liveConfig(), log: filepath.Join(h.logDir, m.Service+".log")}
}

func (h *instanceProcessHost) Systemctl(ctx context.Context, args ...string) (string, error) {
	if args[0] == "daemon-reload" {
		return "", nil
	}
	name := args[len(args)-1]
	h.mu.Lock()
	p := h.hosts[name]
	if args[0] == "restart" && h.failRestart == name {
		h.failRestart = ""
		h.mu.Unlock()
		return "", errors.New("injected restart failure")
	}
	h.mu.Unlock()
	if p == nil {
		return "", errors.New("unknown simulated service")
	}
	return p.Systemctl(ctx, args...)
}

func (h *instanceProcessHost) Run(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func TestTwoLiveInstancesKeepGermanyFlowAcrossBulgariaLifecycle(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("real process fixture requires Linux")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	used := map[int]bool{}
	port := func() int {
		for {
			n := freePort(t)
			if !used[n] {
				used[n] = true
				return n
			}
		}
	}
	mkBase := func(host *instanceProcessHost) *Manager {
		dir := t.TempDir()
		m, err := New(Env{ConfigDir: filepath.Join(dir, "etc"), StateDir: filepath.Join(dir, "state"), UnitDir: filepath.Join(dir, "units"), Service: "baft", BaftBin: baftBin, PairBin: pairBin, Settle: 50 * time.Millisecond, System: host})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	irHost, deHost, bgHost := newInstanceProcessHost(t), newInstanceProcessHost(t), newInstanceProcessHost(t)
	irBase, deBase, bgBase := mkBase(irHost), mkBase(deHost), mkBase(bgHost)
	scope := func(base *Manager, host *instanceProcessHost, id string) *Manager {
		m, err := base.ForInstance(id, "127.0.0.1:"+strconv.Itoa(port()))
		if err != nil {
			t.Fatal(err)
		}
		host.register(m)
		return m
	}
	deIR, deEX := scope(irBase, irHost, "germany"), scope(deBase, deHost, "germany")
	bgIR, bgEX := scope(irBase, irHost, "bulgaria"), scope(bgBase, bgHost, "bulgaria")
	deRoute, bgRoute := "127.0.0.1:"+strconv.Itoa(port()), "127.0.0.1:"+strconv.Itoa(port())
	deTarget, bgTarget := "127.0.0.1:"+strconv.Itoa(echo(t)), "127.0.0.1:"+strconv.Itoa(echo(t))
	var checkGermany func(string)
	build := func(ex, ir *Manager, id, public string, listen int, route, target string, wantFailure bool) {
		check := func(step string) {
			if checkGermany != nil {
				checkGermany(id + ": " + step)
			}
		}
		code, err := ex.PrepareEX(ctx, id, ExParams{PublicAddress: public, Port: listen, Target: target, RouteID: "service-main", VerifyData: true})
		if err != nil {
			t.Fatal(err)
		}
		check("prepare EX")
		reply, err := ir.PrepareIR(ctx, id, code, IRParams{RouteListen: route, RouteID: "service-main", VerifyData: true})
		if err != nil {
			t.Fatal(err)
		}
		check("prepare IR")
		if _, err := ex.CommitEX(ctx, id, reply); err != nil {
			t.Fatal(err)
		}
		check("commit EX")
		_, err = ir.CommitIR(ctx, id)
		if wantFailure {
			if err == nil {
				t.Fatal("injected failure succeeded")
			}
			check("failed commit IR")
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		check("commit IR")
		// Handshake, an active process and an accepting socket cannot pass
		// delivery verification before actual DATA crossed the managed route.
		if p, err := ir.DataEvidence(ctx, id); err != nil || p.State == verification.Pass {
			t.Fatalf("idle runtime was verified: %+v %v", p, err)
		}
		// Type=simple/process-start is not listener readiness. Wait for real
		// handshake and payload before sampling health; a fixed 50ms settle
		// can otherwise race startup on a loaded CI runner.
		through(t, route, id)
		for _, m := range []*Manager{ir, ex} {
			if _, err := m.Health(ctx, id); err != nil {
				t.Fatal(err)
			}
			if p, err := m.DataEvidence(ctx, id); err != nil || p.State != verification.Pass || p.Generation <= 0 {
				t.Fatalf("real payload not verified: %+v %v", p, err)
			}
		}
	}
	finalize := func(ex, ir *Manager, id string) {
		for _, m := range []*Manager{ir, ex} {
			if p, err := m.Seal(ctx, id); err != nil || !p.ReadyToFinalize {
				t.Fatalf("seal: %+v %v", p, err)
			}
		}
		for _, m := range []*Manager{ir, ex} {
			if _, err := m.Finalize(ctx, id); err != nil {
				t.Fatal(err)
			}
		}
	}
	dePort, bgPort := port(), port()
	build(deEX, deIR, "de-first", "127.0.0.1", dePort, deRoute, deTarget, false)
	finalize(deEX, deIR, "de-first")
	flow, err := net.DialTimeout("tcp", deRoute, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer flow.Close()
	files := map[string][]byte{}
	for _, m := range []*Manager{deIR, deEX} {
		for _, p := range []string{m.liveConfig(), m.unitPath(), m.markerPath(), m.generationPath(), m.noiseKey()} {
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			files[p] = b
		}
	}
	proc := irHost.hosts[deIR.Service]
	proc.mu.Lock()
	originalPID, originalGen := proc.cmd.Process.Pid, proc.gen
	proc.mu.Unlock()
	checkGermany = func(step string) {
		t.Helper()
		if err := flow.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
			t.Fatal(err)
		}
		payload := bytes.Repeat([]byte("DE persistent flow: "+step+"\n"), 512)
		if n, err := flow.Write(payload); err != nil || n != len(payload) {
			t.Fatalf("Germany write at %s: %d %v", step, n, err)
		}
		got := make([]byte, len(payload))
		if _, err := io.ReadFull(flow, got); err != nil || !bytes.Equal(got, payload) {
			t.Fatalf("Germany payload lost/corrupt at %s: %v", step, err)
		}
		for p, want := range files {
			b, err := os.ReadFile(p)
			if err != nil || !bytes.Equal(b, want) {
				t.Fatalf("Germany file changed at %s: %s", step, p)
			}
		}
		proc.mu.Lock()
		unchanged := proc.cmd != nil && proc.cmd.Process.Pid == originalPID && proc.gen == originalGen
		proc.mu.Unlock()
		if !unchanged {
			t.Fatalf("Germany process restarted at %s", step)
		}
	}
	checkGermany("baseline")
	build(bgEX, bgIR, "bg-first", "127.0.0.1", bgPort, bgRoute, bgTarget, false)
	finalize(bgEX, bgIR, "bg-first")
	checkGermany("Bulgaria active")
	oldPKI := pkiDigests(bgEX.pkiDir())
	// Changing the public address forces a genuinely different CA/leaf,
	// unlike the earlier port-only regression which reused the same PKI.
	build(bgEX, bgIR, "bg-port-edit", "127.0.0.2", port(), bgRoute, bgTarget, false)
	for _, m := range []*Manager{bgIR, bgEX} {
		if _, err := m.Rollback(ctx, "bg-port-edit"); err != nil {
			t.Fatal(err)
		}
	}
	through(t, bgRoute, "port edit rollback")
	if !sameDigests(pkiDigests(bgEX.pkiDir()), oldPKI) {
		t.Fatal("Bulgaria address/port rollback did not restore its original certificate set")
	}
	checkGermany("Bulgaria port edit rolled back")
	irHost.mu.Lock()
	irHost.failRestart = bgIR.Service
	irHost.mu.Unlock()
	build(bgEX, bgIR, "bg-failed", "127.0.0.1", port(), bgRoute, bgTarget, true)
	for _, m := range []*Manager{bgIR, bgEX} {
		if _, err := m.Rollback(ctx, "bg-failed"); err != nil {
			t.Fatal(err)
		}
	}
	through(t, bgRoute, "failed change rollback")
	checkGermany("Bulgaria failure rolled back")
	for _, m := range []*Manager{bgIR, bgEX} {
		if _, err := m.Retire(ctx, "bg-first", retireWant(m.Inspect(ctx))); err != nil {
			t.Fatal(err)
		}
	}
	checkGermany("Bulgaria deleted")
	t.Log("Germany: same TCP flow, same IR PID/generation and unchanged config/unit/marker/keys across Bulgaria creation, address/port edit with PKI replacement, rollback, injected failure and retirement")
}
