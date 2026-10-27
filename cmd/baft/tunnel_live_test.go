package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This is real loopback transport with configs produced by the standalone CLI.
// systemctl is recorded, not executed: systemd supervision and two physical
// hosts remain separate acceptance gates. Processes are owned by this test.
func TestTerminalAdditionalInstancePreservesLiveFlow(t *testing.T) {
	dir, err := os.MkdirTemp("", "baft-live-") // keep Unix socket paths short
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"baft", "baft-pair"} {
		cmd := exec.CommandContext(ctx, "go", "build", "-o", filepath.Join(bin, name), "./cmd/"+name)
		cmd.Dir = "../.."
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v %s", name, err, out)
		}
	}
	callsPath := filepath.Join(dir, "systemctl.log")
	script := "#!/bin/sh\nprintf '%s %s\\n' \"$BAFT_TEST_ROLE\" \"$*\" >> \"" + callsPath + "\"\n" + `
unit="$BAFT_TEST_SERVICE_STATE/$2"
case "$1" in
is-active) if test -f "$unit.active"; then echo active; else echo inactive; exit 3; fi;;
is-enabled) if test -f "$unit.enabled"; then echo enabled; else echo disabled; exit 1; fi;;
restart) touch "$unit.active";;
stop) rm -f "$unit.active";;
enable) touch "$unit.enabled";;
disable) rm -f "$unit.enabled";;
show) echo 0;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "systemctl"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	// Allocate all candidate addresses together and keep them bound until each
	// external binary needs it. Subprocess listener handoff is unsupported;
	// any bind conflict is a failure, never hidden by a test retry.
	allocate := func() (string, func()) {
		t.Helper()
		ln, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = ln.Close() })
		return ln.Addr().String(), func() { _ = ln.Close() }
	}
	target, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = target.Close() })
	go func() {
		for {
			c, err := target.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
		}
	}()
	type pair struct {
		id, ex, ir, exMetrics, irMetrics                         string
		releaseEX, releaseIR, releaseEXMetrics, releaseIRMetrics func()
	}
	pairs := []*pair{{id: "existing"}, {id: "additional"}, {id: "third"}, {id: "abandoned"}}
	for _, p := range pairs {
		p.ex, p.releaseEX = allocate()
		p.ir, p.releaseIR = allocate()
		p.exMetrics, p.releaseEXMetrics = allocate()
		p.irMetrics, p.releaseIRMetrics = allocate()
	}
	invoke := func(p *pair, role, action string, extra ...string) {
		t.Helper()
		home := filepath.Join(dir, role)
		serviceState := filepath.Join(home, "fake-services")
		if err := os.MkdirAll(serviceState, 0700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("BAFT_TEST_SERVICE_STATE", serviceState)
		t.Setenv("BAFT_TEST_ROLE", role)
		metrics := p.exMetrics
		if role == "ir" {
			metrics = p.irMetrics
		}
		args := []string{action, "--instance", p.id, "--id", "pair-" + p.id,
			"--metrics-listen", metrics, "--config-dir", filepath.Join(home, "etc"),
			"--state-dir", filepath.Join(home, "state"), "--unit-dir", filepath.Join(home, "units"),
			"--bin-dir", bin, "--service-user", ""}
		var out, detail bytes.Buffer
		if status := runTunnelCLI(ctx, append(args, extra...), &out, &detail); status != 0 {
			t.Fatalf("%s %s %s: status=%d %s", p.id, role, action, status, detail.String())
		}
	}
	configPath := func(p *pair, role string) string {
		return filepath.Join(dir, role, "etc", "instances", p.id, "baft.yaml")
	}
	start := func(p *pair, role, address string) func() {
		t.Helper()
		logPath := filepath.Join(dir, p.id+"-"+role+".log")
		log, err := os.Create(logPath)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(ctx, filepath.Join(bin, "baft"), "run", "--file", configPath(p, role))
		cmd.Stdout, cmd.Stderr = log, log
		if err := cmd.Start(); err != nil {
			_ = log.Close()
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		stopped := false
		stop := func() {
			if stopped {
				return
			}
			stopped = true
			_ = cmd.Process.Kill()
			<-done
			_ = log.Close()
		}
		t.Cleanup(stop)
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			c, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
			if err == nil {
				_ = c.Close()
				return stop
			}
			select {
			case err := <-done:
				stopped = true
				_ = log.Close()
				body, _ := os.ReadFile(logPath)
				t.Fatalf("%s %s exited: %v %s", p.id, role, err, body)
			default:
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("%s %s listener did not start", p.id, role)
		return stop
	}
	provision := func(p *pair) []func() {
		t.Helper()
		code, reply := filepath.Join(dir, p.id+".code"), filepath.Join(dir, p.id+".reply")
		_, port, _ := net.SplitHostPort(p.ex)
		invoke(p, "ex", "ex-code", "--address", "127.0.0.1", "--port", port,
			"--target", target.Addr().String(), "--route-id", p.id, "--output-file", code)
		invoke(p, "ir", "ir-apply", "--code-file", code, "--route-listen", p.ir,
			"--route-id", p.id, "--output-file", reply)
		invoke(p, "ex", "ex-accept", "--reply-file", reply)
		p.releaseEX()
		// Keep metrics candidates reserved through provisioning. Releasing them
		// earlier leaves a window where another parallel test can claim the port
		// before the owned runtime process binds it.
		p.releaseEXMetrics()
		stopEX := start(p, "ex", p.ex)
		invoke(p, "ir", "ir-start")
		p.releaseIR()
		p.releaseIRMetrics()
		return []func(){stopEX, start(p, "ir", p.ir)}
	}
	echo := func(c net.Conn, label string) {
		t.Helper()
		_ = c.SetDeadline(time.Now().Add(5 * time.Second))
		payload := bytes.Repeat([]byte(label), 4096)
		if _, err := io.Copy(c, bytes.NewReader(payload)); err != nil {
			t.Fatal(err)
		}
		got := make([]byte, len(payload))
		if _, err := io.ReadFull(c, got); err != nil || !bytes.Equal(got, payload) {
			t.Fatalf("live flow %s changed: %v", label, err)
		}
	}
	provision(pairs[0])
	baselineCalls, err := os.ReadFile(callsPath)
	if err != nil {
		t.Fatal(err)
	}
	flow, err := net.DialTimeout("tcp", pairs[0].ir, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = flow.Close() })
	echo(flow, "before-add")
	// Snapshot all existing config/key/unit/state files before adding a sibling.
	snapshot := map[string][]byte{}
	for _, role := range []string{"ex", "ir"} {
		for _, root := range []string{
			filepath.Dir(configPath(pairs[0], role)),
			filepath.Join(dir, role, "state", "instances", pairs[0].id),
			filepath.Join(dir, role, "units"),
		} {
			if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.Type().IsRegular() {
					body, err := os.ReadFile(path)
					if err != nil {
						return err
					}
					snapshot[path] = body
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	// A pairing started and then cancelled before commit must neither disturb
	// the live flow nor block the instances added after it.
	ab := pairs[3]
	_, abPort, _ := net.SplitHostPort(ab.ex)
	ab.releaseEXMetrics()
	invoke(ab, "ex", "ex-code", "--address", "127.0.0.1", "--port", abPort,
		"--target", target.Addr().String(), "--route-id", ab.id, "--output-file", filepath.Join(dir, ab.id+".code"))
	invoke(ab, "ex", "rollback")
	echo(flow, "after-cancelled-pairing")
	stops := provision(pairs[1])
	echo(flow, "after-add")
	thirdStops := provision(pairs[2])
	echo(flow, "after-third")
	third, err := net.DialTimeout("tcp", pairs[2].ir, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = third.Close() })
	echo(third, "third-instance")
	_ = third.Close()
	second, err := net.DialTimeout("tcp", pairs[1].ir, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	echo(second, "second-instance")
	_ = second.Close()
	// Stop only test-owned second processes, then exercise scoped CLI rollback.
	for _, group := range [][]func(){thirdStops, stops} {
		for i := len(group) - 1; i >= 0; i-- {
			group[i]()
		}
	}
	for _, p := range []*pair{pairs[2], pairs[1]} {
		for _, role := range []string{"ir", "ex"} {
			invoke(p, role, "rollback")
		}
	}
	echo(flow, "after-rollback")
	for path, want := range snapshot {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("existing file changed: %s (%v)", path, err)
		}
	}
	calls, err := os.ReadFile(callsPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(calls)), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[2] == "baft" && mutatesTestService(fields[1]) {
			t.Fatalf("primary service mutated: %s", line)
		}
	}
	if !bytes.HasPrefix(calls, baselineCalls) {
		t.Fatal("service audit log lost baseline")
	}
	for _, line := range strings.Split(string(calls[len(baselineCalls):]), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[2] == "baft-existing" && mutatesTestService(fields[1]) {
			t.Fatalf("sibling operation mutated existing service: %s", line)
		}
	}
	for _, role := range []string{"ex", "ir"} {
		metrics := pairs[0].exMetrics
		if role == "ir" {
			metrics = pairs[0].irMetrics
		}
		_, port, _ := net.SplitHostPort(metrics)
		claim, err := os.ReadFile(filepath.Join(dir, role, "etc", ".metrics-claims", port))
		if err != nil || string(claim) != pairs[0].id+"\n" {
			t.Fatalf("existing metrics claim changed: %v", err)
		}
	}
}

func mutatesTestService(action string) bool {
	switch action {
	case "start", "restart", "reload", "reload-or-restart", "try-restart", "stop", "enable", "disable", "mask", "unmask":
		return true
	}
	return false
}
