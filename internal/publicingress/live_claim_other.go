//go:build !linux

package publicingress

import (
	"context"
	"net"
)

// Linux /proc socket identity is not available on non-Linux laboratory hosts.
// This does not provide whole-port admission authorization on these hosts.
func verifyNoOtherLiveTCPListeners(_ net.Listener, _ int) error { return nil }

// The strict kernel live-port watchdog is Linux-only. Other platforms are
// allowed for local LAB testing but do not prove full port exclusivity.
func watchLiveTCPListener(_ context.Context, _ *Forwarder, _ net.Listener, _ chan<- error) <-chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}
