package release_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Tests the actual CLI's fail-closed pairing-code path. This does not certify
// real two-host reachability, SSH blocking or systemd.
func TestGammaRejectedPairCodeDoesNotCreateIRState(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil { t.Fatal(err) }
	work := t.TempDir()
	bin := filepath.Join(work, "baft-pair")
	build := exec.Command("go", "build", "-o", bin, "./cmd/baft-pair")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil { t.Fatalf("build baft-pair: %v: %s", err, out) }
	exDir := filepath.Join(work, "ex")
	if err := os.Mkdir(exDir, 0o700); err != nil { t.Fatal(err) }
	ca := filepath.Join(exDir, "ca.pem")
	// Expired codes are rejected before CA parsing; the full PKI flow is
	// covered by the separately executed tests/e2e/pair_and_run.sh.
	if err := os.WriteFile(ca, []byte("placeholder-CA"), 0o600); err != nil { t.Fatal(err) }
	codeCmd := exec.Command(bin, "ex-code",
		"--key", filepath.Join(exDir, "ex.key"),
		"--address", "127.0.0.1:443",
		"--server-name", "127.0.0.1",
		"--identity", "urn:baft:node:ex-gamma",
		"--ca-file", ca,
		"--psk-out", filepath.Join(exDir, "pair.psk"),
		"--ttl", "1s")
	raw, err := codeCmd.Output()
	if err != nil { t.Fatalf("create short-lived EX code: %v", err) }
	code := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(code, "BAFTPAIR1:") { t.Fatal("expected BAFTPAIR1 descriptor") }
	// Unix-second expiration: waiting over one second ensures rejection.
	time.Sleep(1200 * time.Millisecond)

	cases := []struct{name, code, reason string}{
		{"expired", code, "expired"},
		{"bad-prefix", "BAFTPAIR0:not-a-valid-code", "prefix"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(work, tc.name)
			key := filepath.Join(dir, "ir.key")
			state := filepath.Join(dir, "state")
			config := filepath.Join(dir, "ir.yaml")
			cmd := exec.Command(bin, "ir-apply", "--code", tc.code,
				"--key", key, "--state-dir", state, "--config-out", config)
			output, err := cmd.CombinedOutput()
			if err == nil { t.Fatal("invalid code accepted") }
			if !strings.Contains(strings.ToLower(string(output)), tc.reason) {
				t.Fatalf("expected %q rejection, got: %s", tc.reason, output)
			}
			for _, path := range []string{key,state,config} {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatalf("invalid code modified %s: %v", filepath.Base(path), err)
				}
			}
		})
	}
}
