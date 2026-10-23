//go:build !linux

package publicingress

import "net"

// Linux /proc socket identity is not available on non-Linux laboratory hosts.
// This does not provide whole-port admission authorization on these hosts.
func verifyNoOtherLiveTCPListeners(_ net.Listener, _ int) error { return nil }
