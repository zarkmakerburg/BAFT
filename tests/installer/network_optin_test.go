package installer_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNetworkSetupRequiresExplicitConsent(t *testing.T) {
	raw, err := os.ReadFile("../../install.sh")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	start := strings.Index(source, "\nnet_setup() {")
	if start < 0 {
		t.Fatal("net_setup missing")
	}
	function := source[start+1:]
	end := strings.Index(function, "\n}\n")
	if end < 0 {
		t.Fatal("net_setup terminator missing")
	}
	function = function[:end+2]
	for _, tc := range []struct {
		name, answer, tune, tty, fail string
		existing, want                bool
	}{
		{name: "default-no", tty: "1"},
		{name: "eof", tty: "1", fail: "1"},
		{name: "no", answer: "n", tty: "1"},
		{name: "invalid", answer: "maybe", tty: "1"},
		{name: "yes", answer: "yes", tty: "1", want: true},
		{name: "y", answer: "y", tty: "1", want: true},
		{name: "Y", answer: "Y", tty: "1", want: true},
		{name: "explicit-env", tune: "1", tty: "0", want: true},
		{name: "disabled-env", tune: "0", answer: "y", tty: "1"},
		{name: "noninteractive", tty: "0"},
		{name: "already-managed", tune: "1", tty: "0", existing: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			marker := filepath.Join(dir, "called")
			profile := filepath.Join(dir, "sysctl.conf")
			if tc.existing {
				if err := os.WriteFile(profile, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			harness := `
write_net_tool(){ printf '%s\n' '#!/bin/bash' 'printf on > "$CALL_MARKER"' > "$1"; chmod 700 "$1"; }
ask(){ [[ "$FAIL_ASK" != 1 ]] || return 1; printf -v "$1" '%s' "$ANSWER"; }
ui_ask_prompt(){ :; }; ok(){ :; }; warn(){ :; }; sysctl(){ echo bbr; }
net_setup "$TEST_DIR"
`
			cmd := exec.Command("bash", "-c", "set -euo pipefail\n"+function+harness)
			cmd.Env = append(os.Environ(), "BAFT_NET_SYSCTL="+profile, "BAFT_TUNE_NETWORK="+tc.tune, "ASK_TTY="+tc.tty, "FAIL_ASK="+tc.fail, "ANSWER="+tc.answer, "TEST_DIR="+dir, "CALL_MARKER="+marker)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%v: %s", err, out)
			}
			_, err = os.Stat(marker)
			called := err == nil
			if called != tc.want {
				t.Fatalf("applied=%v want %v", called, tc.want)
			}
		})
	}
}
