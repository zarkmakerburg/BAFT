package bcc

import (
	"strings"
	"testing"
)

// Independent tunnel creation cannot steal an EX listener port. Explicit
// JoinExistingTunnelID is a separate, reviewed path that authenticates and
// isolates each tenant on the existing listener without weakening this guard.
func TestP0SharedTransportRequiresMultiplexer(t *testing.T) {
	a := Tunnel{ID: "a", InstanceID: "instance-a", IRNode: "arno", EXNode: "bg", Port: 443, RouteListen: "127.0.0.1:1444", IRMetricsListen: "127.0.0.1:9192", EXMetricsListen: "127.0.0.1:9193"}
	b := Tunnel{ID: "b", InstanceID: "instance-b", IRNode: "arno", EXNode: "bg", Port: 443, RouteListen: "127.0.0.1:1445", IRMetricsListen: "127.0.0.1:9194", EXMetricsListen: "127.0.0.1:9195"}
	if got := tunnelResourceConflict(a, b); !strings.Contains(got, "EX listener port 443") {
		t.Fatalf("independent tunnels must not silently share TCP/443 without multiplexing, got %q", got)
	}
	b.Port = 8443
	if got := tunnelResourceConflict(a, b); got != "" {
		t.Fatalf("independent nonconflicting ports should be allowed, got %q", got)
	}
}
