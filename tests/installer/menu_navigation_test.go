package installer_test

import (
	"os/exec"
	"runtime"
	"testing"
)

func TestGeneratedManagementMenuPTY(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX PTY required")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("python3 required for installer acceptance: ", err)
	}
	cmd := exec.Command(python, "testdata/menu_navigation.py", "-v")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated menu PTY: %v\n%s", err, out)
	}
}
