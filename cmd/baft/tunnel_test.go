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
	for _, role := range []string{"ex", "ir"} {
		if _, err := os.Stat(filepath.Join(dir, role, "etc", "instances", "standalone-1", "noise-key.json")); err != nil {
			t.Fatal(err)
		}
	}
}
