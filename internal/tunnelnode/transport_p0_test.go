package tunnelnode

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// The EX listener, IR route listener and metrics endpoint must never land on
// SSH management (22) or collide with each other. These are refused before
// any state is written, host service is touched, or pairing secret is issued.
func TestPrepareRefusesProtectedAndConflictingPorts(t *testing.T) {
	ctx := context.Background()
	good := ExParams{PublicAddress: "203.0.113.7", Port: 8443, Target: "127.0.0.1:2443", RouteID: "r1", MetricsListen: "127.0.0.1:9300"}

	bad := map[string]func(*ExParams){
		"EX listener on SSH port 22":         func(p *ExParams) { p.Port = 22 },
		"EX listener equals metrics port":    func(p *ExParams) { p.Port = 9300 },
		"metrics listener on SSH port 22":    func(p *ExParams) { p.MetricsListen = "127.0.0.1:22" },
		"EX listener equals default metrics": func(p *ExParams) { p.MetricsListen = ""; p.Port = 9191 },
	}
	for name, mutate := range bad {
		n := newNode(t)
		p := good
		mutate(&p)
		if _, err := n.PrepareEX(ctx, "v1", p); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if _, err := os.Stat(filepath.Join(n.StateDir, "tunnels")); err == nil {
			t.Errorf("%s: left state behind", name)
		}
		if n.host.called("restart") || n.host.called("enable") {
			t.Errorf("%s: touched the host service", name)
		}
	}

	n := newNode(t)
	if _, err := n.PrepareIR(ctx, "v2", "BAFTPAIR1:x", IRParams{RouteListen: "127.0.0.1:22", RouteID: "r1"}); err == nil {
		t.Error("IR route listener on SSH port 22 accepted")
	}
	if _, err := n.PrepareIR(ctx, "v3", "BAFTPAIR1:x", IRParams{RouteListen: "127.0.0.1:9191", RouteID: "r1"}); err == nil {
		t.Error("IR route listener equal to default metrics port accepted")
	}
	if _, err := os.Stat(filepath.Join(n.StateDir, "tunnels")); err == nil {
		t.Error("an invalid IR request left state behind")
	}
}
