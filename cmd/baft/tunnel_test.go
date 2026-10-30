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
	// Simulate an installed primary transport on both hosts. Scoped CLI
	// operations must preserve all primary files byte-for-byte.
	primaries := map[string][]byte{}
	for _, role := range []string{"ex", "ir"} {
		home := filepath.Join(dir, role)
		for rel, body := range map[string]string{
			"etc/baft.yaml":       "primary-live-config\n",
			"etc/noise-key.json":  "primary-private-key\n",
			"units/baft.service":  "primary-systemd-unit\n",
			"state/primary-state": "primary-live-state\n",
		} {
			path := filepath.Join(home, rel)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			primaries[path] = []byte(body)
		}
	}
	codePath := filepath.Join(dir, "ex-pairing-code")
	replyPath := filepath.Join(dir, "ir-pairing-reply")
	codeOut, status, detail := invoke("ex", "ex-code", "--address", "127.0.0.1", "--port", "8443", "--target", "127.0.0.1:1443", "--route-id", "route-1", "--output-file", codePath)
	if status != 0 || strings.Contains(codeOut, "BAFTPAIR1:") {
		t.Fatalf("EX code status=%d output=%s detail=%s", status, codeOut, detail)
	}
	code, err := os.ReadFile(codePath)
	if err != nil || !strings.HasPrefix(string(code), "BAFTPAIR1:") {
		t.Fatalf("bad stored code: %v", err)
	}
	replyOut, status, detail := invoke("ir", "ir-apply", "--code-file", codePath, "--route-id", "route-1", "--route-listen", "127.0.0.1:15443", "--output-file", replyPath)
	if status != 0 || strings.Contains(replyOut, "BAFTREPLY1:") {
		t.Fatalf("IR apply status=%d output=%s detail=%s", status, replyOut, detail)
	}
	reply, err := os.ReadFile(replyPath)
	if err != nil || !strings.HasPrefix(string(reply), "BAFTREPLY1:") {
		t.Fatalf("bad stored reply: %v", err)
	}
	for _, secret := range []string{codePath, replyPath} {
		info, err := os.Stat(secret)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("insecure secret permissions: %s %v", secret, err)
		}
	}
	if output, status, detail := invoke("ex", "ex-accept", "--reply-file", replyPath); status != 0 {
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
	for path, want := range primaries {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("primary transport mutated: %s (%v)", path, err)
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

func TestTerminalTunnelInspectReadOnlyWithoutTransactionID(t *testing.T) {
	dir := t.TempDir()
	args := []string{"inspect", "--instance", "isolated", "--metrics-listen", "127.0.0.1:9421", "--config-dir", filepath.Join(dir, "etc"), "--state-dir", filepath.Join(dir, "state"), "--unit-dir", filepath.Join(dir, "units")}
	var out, stderr bytes.Buffer
	if status := runTunnelCLI(context.Background(), args, &out, &stderr); status != 0 {
		t.Fatalf("inspect failed: %d %s", status, stderr.String())
	}
	if !json.Valid(out.Bytes()) {
		t.Fatalf("inspect output not JSON: %s", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "etc", ".metrics-claims")); !os.IsNotExist(err) {
		t.Fatalf("inspect unexpectedly reserved a port: %v", err)
	}
}
