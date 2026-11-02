package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMultiCallSubcommandsMatchLegacyEntrypoints(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "baft")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build multicall binary: %v\n%s", err, out)
	}

	for _, name := range []string{"baft-agent", "baft-bcc", "baft-pair"} {
		if err := os.Symlink(binary, filepath.Join(dir, name)); err != nil {
			t.Fatalf("symlink %s: %v", name, err)
		}
	}

	cases := []struct {
		name       string
		legacyName string
		subcommand []string
		legacyArgs []string
		wantFlag   string
	}{
		{name: "agent", legacyName: "baft-agent", subcommand: []string{"agent", "--help"}, legacyArgs: []string{"--help"}, wantFlag: "-bcc-url"},
		{name: "panel", legacyName: "baft-bcc", subcommand: []string{"panel", "--help"}, legacyArgs: []string{"--help"}, wantFlag: "-admin-token-file"},
		{name: "pair", legacyName: "baft-pair", subcommand: []string{"pair", "keygen", "--help"}, legacyArgs: []string{"keygen", "--help"}, wantFlag: "-file"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			subCode, subOutput := runMultiCallCommand(binary, tc.subcommand...)
			legacyCode, legacyOutput := runMultiCallCommand(filepath.Join(dir, tc.legacyName), tc.legacyArgs...)
			if subCode != legacyCode {
				t.Fatalf("exit code differs: subcommand=%d legacy=%d\nsubcommand output:\n%s\nlegacy output:\n%s", subCode, legacyCode, subOutput, legacyOutput)
			}
			if !strings.Contains(subOutput, tc.wantFlag) || !strings.Contains(legacyOutput, tc.wantFlag) {
				t.Fatalf("help output missing %q\nsubcommand output:\n%s\nlegacy output:\n%s", tc.wantFlag, subOutput, legacyOutput)
			}
		})
	}
}

func runMultiCallCommand(binary string, args ...string) (int, string) {
	cmd := exec.Command(binary, args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), string(out)
	}
	return -1, string(out)
}
