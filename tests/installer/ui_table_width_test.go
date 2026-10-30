package installer_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestInstallerPlanTableFitsNarrowTerminals(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash unavailable")
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(string(raw), "need_root(){", 2)
	if len(parts) != 2 {
		t.Fatal("installer UI helper boundary missing")
	}
	helper := filepath.Join(t.TempDir(), "ui.sh")
	if err := os.WriteFile(helper, []byte(parts[0]), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{40, 60, 80} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			cmd := exec.Command("bash", "-c", `source "$1"; UI=1; COLUMNS="$2"; ui_table 'Change|Action|Detail' 'Native BAFT service|@warn:replace|A long but readable plan detail with a verylongunbrokentokenthatmustwrapwithoutoverflow'`, "bash", helper, strconv.Itoa(width))
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("table: %v %s", err, out)
			}
			for _, line := range strings.Split(string(out), "\n") {
				if got := utf8.RuneCountInString(line); got > width {
					t.Fatalf("line width %d > %d: %q", got, width, line)
				}
			}
			if width < 64 && strings.Contains(string(out), "┌") {
				t.Fatal("narrow table retained box drawing")
			}
			if !strings.Contains(string(out), "Native BAFT service") || !strings.Contains(string(out), "replace") {
				t.Fatalf("plan facts missing: %s", out)
			}
		})
	}
}
