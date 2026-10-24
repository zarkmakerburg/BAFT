//go:build !linux

package ingressownership

import (
	"net"
	"testing"
)

// Non-Linux has no /proc socket inode inventory; File() closure is still
// checked in the platform-independent wildcard retirement test.
func ownedListenerKernelClosureCheck(_ *testing.T, _ *net.TCPListener) func() {
	return func() {}
}
