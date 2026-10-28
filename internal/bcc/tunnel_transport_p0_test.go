package bcc

import (
	"strings"
	"testing"
	"time"
)

// The EX listener port is chosen by the operator/wizard; the planner must
// refuse SSH management, out-of-range and metrics-aliasing ports before any
// job or intent exists, and a refused plan must leave no durable tunnel.
func TestCreateTunnelRefusesProtectedAndInvalidListenerPorts(t *testing.T) {
	cases := []struct {
		name string
		req  TunnelRequest
		want string
	}{
		{"ssh management", TunnelRequest{EXNode: "ex-1", IRNode: "ir-1", Port: 22}, "22"},
		{"negative", TunnelRequest{EXNode: "ex-1", IRNode: "ir-1", Port: -1}, "port"},
		{"above 65535", TunnelRequest{EXNode: "ex-1", IRNode: "ir-1", Port: 65536}, "port"},
		{"equals EX metrics", TunnelRequest{EXNode: "ex-1", IRNode: "ir-1", InstanceID: "inst-a", Port: 9300,
			EXMetricsListen: "127.0.0.1:9300", IRMetricsListen: "127.0.0.1:9301"}, "metrics"},
	}
	for _, tc := range cases {
		s := tunnelStore(t)
		now := time.Now()
		if _, err := s.CreateTunnel(tc.req, now); err == nil {
			t.Errorf("%s: accepted", tc.name)
		} else if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: unexpected refusal %q", tc.name, err)
		}
		if got := len(pullAll(t, s, "ex-1")) + len(pullAll(t, s, "ir-1")); got != 0 {
			t.Errorf("%s: refused request left %d jobs", tc.name, got)
		}
	}
	s := tunnelStore(t)
	if _, err := s.CreateTunnel(TunnelRequest{EXNode: "ex-1", IRNode: "ir-1", Port: 443}, time.Now()); err != nil {
		t.Fatalf("an ordinary privileged non-SSH port must stay valid: %v", err)
	}
}
