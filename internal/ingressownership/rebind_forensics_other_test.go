//go:build !linux

package ingressownership

import (
	"net"
	"testing"
)

// Non-Linux has no /proc TCP tables; behave exactly like net.Listen.
func listenWithForensics(t *testing.T, network, address string) (net.Listener, error) {
	t.Helper()
	return net.Listen(network, address)
}
