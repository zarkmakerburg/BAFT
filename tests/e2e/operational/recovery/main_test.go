//go:build linux

package recovery

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/zarkmakerburg/baft/tests/e2e/operational/oplab"
)

// labBin is the recoverylab program built once for every test.
var labBin string

const gateEnv = "BAFT_OPERATIONAL_RECOVERY"

func enabled() bool { return os.Getenv(gateEnv) == "1" }

// TestMain: with BAFT_OPERATIONAL_RECOVERY=1 the process must already be in a
// private MOUNT namespace (the workflow starts it under `unshare -m`); it then
// gives that namespace a tmpfs copy of /etc so the installed policy written by
// the lab survives every SIGKILLed incarnation and never touches the host.
func TestMain(m *testing.M) {
	if enabled() {
		if err := oplab.PrepareMounts(); err != nil {
			fmt.Fprintln(os.Stderr, "recovery lab: private mount namespace:", err)
			os.Exit(1)
		}
		dir, err := os.MkdirTemp("/dev/shm", "oprec-bin-")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		labBin = filepath.Join(dir, "recoverylab")
		cmd := exec.Command("go", "build", "-o", labBin, "./tests/e2e/operational/recoverylab")
		cmd.Dir = "../../../.."
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "build recoverylab: %v\n%s", err, out)
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}
