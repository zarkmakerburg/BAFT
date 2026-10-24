// Package oplab is the shared LAB support for the operational-ingress-v1 gate
// tests (see oplab_linux.go). It is imported only by _test.go files.
package oplab

import "os"

const (
	// GateEnv is set by tests/release/opgate/run_lab.sh. When it is set a
	// missing prerequisite FAILS (the gate counts SKIP as red).
	GateEnv = "BAFT_OPERATIONAL_INGRESS_V1"
	// NFT is the only nft binary path the production runner accepts.
	NFT = "/usr/sbin/nft"
)

// Gate reports whether the pinned gate runner is executing the tests.
func Gate() bool { return os.Getenv(GateEnv) == "1" }
