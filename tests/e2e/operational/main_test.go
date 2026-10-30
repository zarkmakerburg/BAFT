//go:build linux

package operational

import (
	"os"
	"testing"

	"github.com/zarkmakerburg/baft/tests/e2e/operational/oplab"
)

func TestMain(m *testing.M) { os.Exit(oplab.Main(m)) }
