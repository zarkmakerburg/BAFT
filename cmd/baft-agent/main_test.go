package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFlagsAndSecrets(t *testing.T) {
	var out bytes.Buffer
	if code := run(context.Background(), []string{"--node-id", "ex-1"}, &out); code != 2 {
		t.Fatalf("missing --bcc-url = %d", code)
	}
	out.Reset()
	if code := run(context.Background(), []string{"--bcc-url", "http://bcc", "--node-id", "ex-1"}, &out); code != 2 || !strings.Contains(out.String(), "https") {
		t.Fatalf("plain HTTP accepted: %d %s", code, out.String())
	}
	dir := t.TempDir()
	tok := filepath.Join(dir, "token")
	if err := os.WriteFile(tok, []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Explicit chmod is necessary because the host umask may mask 0644
	// during creation, silently invalidating the negative permission test.
	if err := os.Chmod(tok, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readSecret(tok); err == nil {
		t.Fatal("group-readable token accepted")
	}
	if err := os.Chmod(tok, 0o600); err != nil {
		t.Fatal(err)
	}
	if v, err := readSecret(tok); err != nil || v != "secret" {
		t.Fatalf("token = %q, %v", v, err)
	}
}

func TestProvisionIngressPolicyRequiresExplicitNode(t *testing.T) {
	var out bytes.Buffer
	code := run(context.Background(), []string{"--provision-ingress-policy"}, &out)
	if code != 2 || !strings.Contains(out.String(), "requires --node-id") {
		t.Fatalf("local policy bootstrap did not validate its node identity: code=%d %s", code, out.String())
	}
}

func TestProvisionIngressPolicyHonorsCancellationWithoutBCCCredentials(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	code := run(ctx, []string{"--provision-ingress-policy", "--node-id", "ir-canceled"}, &out)
	if code != 1 || !strings.Contains(out.String(), "ingress policy:") || strings.Contains(out.String(), "--bcc-url and --node-id are required") {
		t.Fatalf("local policy bootstrap did not handle cancellation before credentials/polling: code=%d %s", code, out.String())
	}
}
