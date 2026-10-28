//go:build linux

package ingressauthority

import (
	"os"
	"testing"

	"github.com/zarkmakerburg/baft/tests/e2e/operational/oplab"
)

// TestMain gives the operational-ingress-v1 gate tests of this package their
// private mount namespace (a no-op unless the gate runner sets its env).
func TestMain(m *testing.M) { os.Exit(oplab.Main(m)) }
