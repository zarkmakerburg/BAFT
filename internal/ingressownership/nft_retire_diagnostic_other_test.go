//go:build !linux

package ingressownership

import (
	"net"
	"testing"
)

func captureRetireRebindDiagnostic(_ *testing.T, _ *net.TCPListener) func() string {
	return func() string { return "Linux procfs socket diagnostics unavailable on this platform" }
}
