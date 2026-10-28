package tunnelnode

import (
	"context"
	"os"
	"strings"
	"testing"
)

func exParams(t *testing.T) ExParams {
	return ExParams{PublicAddress: "127.0.0.1", Port: freePort(t), Target: "127.0.0.1:2443", RouteID: "r"}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// Ownership: a config or unit that BAFT did not write (no marker) sits where a
// new scoped instance would install. Pairing must refuse instead of replacing it.
func TestPairingRefusesForeignConfigAndUnitInInstanceNamespace(t *testing.T) {
	ctx := context.Background()
	for name, setup := range map[string]func(m *Manager){
		"config": func(m *Manager) {
			must(t, os.MkdirAll(m.ConfigDir, 0o750))
			must(t, os.WriteFile(m.liveConfig(), []byte("foreign: config\n"), 0o640))
		},
		"unit": func(m *Manager) {
			must(t, os.MkdirAll(m.UnitDir, 0o755))
			must(t, os.WriteFile(m.unitPath(), []byte("[Service]\nExecStart=/bin/true\n"), 0o644))
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := newPair(t)
			ex := scoped(t, p.ex, "mine", "127.0.0.1:9831")
			ir := scoped(t, p.ir, "mine", "127.0.0.1:9832")
			setup(ex)
			code, err := ex.PrepareEX(ctx, "p1", p.exp)
			if err != nil {
				return // refused at prepare: fail closed
			}
			reply, err := ir.PrepareIR(ctx, "p1", code, p.irp)
			must(t, err)
			if _, err := ex.CommitEX(ctx, "p1", reply); err == nil {
				t.Fatal("commit replaced a foreign " + name)
			}
			if name == "config" {
				if b, _ := os.ReadFile(ex.liveConfig()); string(b) != "foreign: config\n" {
					t.Fatalf("foreign config modified: %q", b)
				}
			} else if b, _ := os.ReadFile(ex.unitPath()); !strings.Contains(string(b), "/bin/true") {
				t.Fatalf("foreign unit modified: %q", b)
			}
		})
	}
}

// Routing: a new instance may not claim a public listener or an IR route
// listener that a sibling's committed config already owns, even when the
// sibling service is currently stopped (it would lose its port on restart).
func TestNewInstanceRefusesSiblingListenerPorts(t *testing.T) {
	ctx := context.Background()
	p := newPair(t)
	exA := scoped(t, p.ex, "a", "127.0.0.1:9841")
	irA := scoped(t, p.ir, "a", "127.0.0.1:9842")
	listen(t, p.exPort)
	listen(t, mustPort(p.irListen))
	code, err := exA.PrepareEX(ctx, "t1", p.exp)
	must(t, err)
	reply, err := irA.PrepareIR(ctx, "t1", code, p.irp)
	must(t, err)
	_, err = exA.CommitEX(ctx, "t1", reply)
	must(t, err)
	_, err = irA.CommitIR(ctx, "t1")
	must(t, err)
	_, err = exA.Finalize(ctx, "t1")
	must(t, err)
	_, err = irA.Finalize(ctx, "t1")
	must(t, err)
	exB := scoped(t, p.ex, "b", "127.0.0.1:9843")
	irB := scoped(t, p.ir, "b", "127.0.0.1:9844")
	if _, err := exB.PrepareEX(ctx, "t2", ExParams{PublicAddress: "127.0.0.1", Port: p.exPort, Target: "127.0.0.1:2443", RouteID: "r"}); err == nil {
		t.Fatal("second EX instance accepted the sibling's public port")
	}
	codeB, err := exB.PrepareEX(ctx, "t3", exParams(t))
	must(t, err)
	if _, err := irB.PrepareIR(ctx, "t3", codeB, IRParams{RouteListen: p.irListen, RouteID: "r"}); err == nil {
		t.Fatal("second IR instance accepted the sibling's route listener")
	}
}
