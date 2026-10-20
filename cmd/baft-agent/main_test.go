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
