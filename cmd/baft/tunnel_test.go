package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTerminalTunnelRequiresScopedInstance(t *testing.T) {
	for _, args := range [][]string{
		{"ex-code"},
		{"ex-code", "--instance", "default", "--metrics-listen", "127.0.0.1:9201", "--id", "new"},
		{"ex-code", "--instance", "other", "--metrics-listen", "127.0.0.1:9201", "--id", "new", "--address", "127.0.0.1"},
		{"ir-apply", "--instance", "other", "--metrics-listen", "127.0.0.1:9201", "--id", "new"},
		{"ex-accept", "--instance", "other", "--metrics-listen", "127.0.0.1:9201", "--id", "new"},
	} {
		var out, stderr bytes.Buffer
		if got := runTunnelCLI(context.Background(), args, &out, &stderr); got != 2 {
			t.Errorf("args %v returned %d, want usage failure: %s", args, got, stderr.String())
		}
		if strings.Contains(out.String(), "BAFTPAIR1:") {
			t.Fatal("pairing code returned on rejected call")
		}
	}
}

func TestTerminalPairingCodeRoundTripWithoutBCC(t *testing.T) {
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
	// A fake systemctl records every call while never starting host services.
	// This integration test exercises config and unit commits only; it does not
	// prove socket ownership or network availability on live Linux nodes.
	serviceLog := filepath.Join(dir, "systemctl.log")
	serviceScript := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"" + serviceLog + "\"\ncase \"$1\" in is-active) echo active;; is-enabled) echo enabled;; show) echo 0;; esac\n"
	if err := os.WriteFile(filepath.Join(bin, "systemctl"), []byte(serviceScript), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	invoke := func(role, action string, extras ...string) (string, int, string) {
		home := filepath.Join(dir, role)
		args := []string{action, "--instance", "standalone-1", "--metrics-listen", "127.0.0.1:9321", "--id", "pair-1", "--config-dir", filepath.Join(home, "etc"), "--state-dir", filepath.Join(home, "state"), "--unit-dir", filepath.Join(home, "units"), "--bin-dir", bin, "--service-user", ""}
		args = append(args, extras...)
		var out, errors bytes.Buffer
		status := runTunnelCLI(ctx, args, &out, &errors)
		return strings.TrimSpace(out.String()), status, errors.String()
	}
	code, status, detail := invoke("ex", "ex-code", "--address", "127.0.0.1", "--port", "8443", "--target", "127.0.0.1:1443", "--route-id", "route-1")
	if status != 0 || !strings.HasPrefix(code, "BAFTPAIR1:") {
		t.Fatalf("EX code status=%d detail=%s", status, detail)
	}
	reply, status, detail := invoke("ir", "ir-apply", "--code", code, "--route-id", "route-1", "--route-listen", "127.0.0.1:15443")
	if status != 0 || !strings.HasPrefix(reply, "BAFTREPLY1:") {
		t.Fatalf("IR apply status=%d detail=%s", status, detail)
	}
	if output, status, detail := invoke("ex", "ex-accept", "--reply", reply); status != 0 {
		t.Fatalf("EX accept: %d %s %s", status, detail, output)
	}
	if output, status, detail := invoke("ir", "ir-start"); status != 0 {
		t.Fatalf("IR start: %d %s %s", status, detail, output)
	}
	for _, role := range []string{"ex", "ir"} {
		if output, status, detail := invoke(role, "rollback"); status != 0 {
			t.Fatalf("%s rollback: status=%d output=%s detail=%s", role, status, output, detail)
		}
		claim := filepath.Join(dir, role, "etc", ".metrics-claims", "9321")
		if _, err := os.Stat(claim); !os.IsNotExist(err) {
			t.Fatalf("%s rollback did not release owned metrics claim: %v", role, err)
		}
	}
	calls, err := os.ReadFile(serviceLog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(calls), "restart baft-standalone-1") || strings.Contains(string(calls), "restart baft\n") {
		t.Fatalf("unsafe service operation: %s", calls)
	}
	for _, role := range []string{"ex", "ir"} {
		if _, err := os.Stat(filepath.Join(dir, role, "etc", "instances", "standalone-1", "noise-key.json")); err != nil {
			t.Fatal(err)
		}
	}
}
