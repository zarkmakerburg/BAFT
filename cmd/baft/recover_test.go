package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zarkmakerburg/baft/internal/tunnelnode"
)

func TestRecoverBootUsageAndPathValidation(t *testing.T) {
	for _, args := range [][]string{{}, {"nope"}, {"boot", "--config-dir", "relative"}, {"boot", "extra"}, {"boot", "--state-dir", "/var/../lib"}} {
		var out, errb bytes.Buffer
		if got := runRecover(context.Background(), args, &out, &errb); got != 2 {
			t.Errorf("%v returned %d, want usage failure (%s)", args, got, errb.String())
		}
	}
}

func TestRecoverBootEmptyNodeIsANoop(t *testing.T) {
	dir := t.TempDir()
	var out, errb bytes.Buffer
	args := []string{"boot", "--json", "--config-dir", filepath.Join(dir, "etc"), "--state-dir", filepath.Join(dir, "state"), "--unit-dir", filepath.Join(dir, "units"), "--bin-dir", filepath.Join(dir, "bin")}
	if got := runRecover(context.Background(), args, &out, &errb); got != 0 {
		t.Fatalf("exit %d: %s", got, errb.String())
	}
	var reports []tunnelnode.BootReport
	if err := json.Unmarshal(out.Bytes(), &reports); err != nil || len(reports) != 1 || reports[0].Action != tunnelnode.BootNone {
		t.Fatalf("reports: %s %v", out.String(), err)
	}
}

// An interrupted pairing (only the EX code was made) is rolled back at boot by
// the real command, and a foreign file next to it is untouched.
func TestRecoverBootRollsBackAnInterruptedPairing(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	for _, binary := range []string{"baft", "baft-pair"} {
		cmd := exec.Command("go", "build", "-o", filepath.Join(bin, binary), "./cmd/"+binary)
		cmd.Dir = "../.."
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v %s", binary, err, out)
		}
	}
	script := "#!/bin/sh\ncase \"$1\" in is-active) echo inactive;; is-enabled) echo disabled;; show) echo 0;; esac\n"
	if err := os.WriteFile(filepath.Join(bin, "systemctl"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	etc, state, units := filepath.Join(dir, "etc"), filepath.Join(dir, "state"), filepath.Join(dir, "units")
	var out, errb bytes.Buffer
	if got := runTunnelCLI(ctx, []string{"ex-code", "--instance", "standalone-1", "--metrics-listen", "127.0.0.1:9322", "--id", "pair-9",
		"--config-dir", etc, "--state-dir", state, "--unit-dir", units, "--bin-dir", bin, "--service-user", "",
		"--address", "127.0.0.1", "--port", "8444", "--target", "127.0.0.1:1443", "--route-id", "route-1", "--output-file", filepath.Join(dir, "code")}, &out, &errb); got != 0 {
		t.Fatalf("ex-code: %d %s", got, errb.String())
	}
	foreign := filepath.Join(units, "x-ui.service")
	if err := os.MkdirAll(units, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(foreign, []byte("[Service]\nExecStart=/usr/bin/x-ui\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	active := filepath.Join(state, "instances", "standalone-1", "tunnels", "active")
	if _, err := os.Stat(active); err != nil {
		t.Fatalf("fixture: no active marker: %v", err)
	}
	out.Reset()
	errb.Reset()
	args := []string{"boot", "--config-dir", etc, "--state-dir", state, "--unit-dir", units, "--bin-dir", bin, "--service-user", ""}
	if got := runRecover(ctx, args, &out, &errb); got != 0 {
		t.Fatalf("recover: %d %s %s", got, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "instance=standalone-1 action=rolled_back change=pair-9") {
		t.Fatalf("unexpected report: %s", out.String())
	}
	if _, err := os.Stat(active); !os.IsNotExist(err) {
		t.Fatal("node still locked after boot recovery")
	}
	if b, err := os.ReadFile(foreign); err != nil || !strings.Contains(string(b), "x-ui") {
		t.Fatalf("foreign unit touched: %v", err)
	}
}
