package tunnelnode

import (
	"context"
	"os"
	"testing"
)

// BCC comes back after the node was paired from the terminal: it must see the
// terminal-created scoped instance as BAFT-owned and consistent, may retire it
// with its own accepted proof, and the same instance name and metrics port are
// then reusable for a fresh pairing (the retired instance's claim is not
// leaked).
func TestBCCHandoffAfterTerminalPairingThenRetireAndRepair(t *testing.T) {
	ctx := context.Background()
	p := newPair(t)
	listen(t, p.exPort)
	listen(t, mustPort(p.irListen))
	ex, ir := scoped(t, p.ex, "term", "127.0.0.1:9871"), scoped(t, p.ir, "term", "127.0.0.1:9872")
	code, err := ex.PrepareEX(ctx, "t1", p.exp)
	must(t, err)
	reply, err := ir.PrepareIR(ctx, "t1", code, p.irp)
	must(t, err)
	_, err = ex.CommitEX(ctx, "t1", reply)
	must(t, err)
	_, err = ir.CommitIR(ctx, "t1")
	must(t, err)
	for _, m := range []*Manager{ex, ir} {
		_, err := m.Finalize(ctx, "t1")
		must(t, err)
	}
	for name, m := range map[string]*Manager{"ex": ex, "ir": ir} {
		fresh := freshManager(t, map[string]*node{"ex": p.ex, "ir": p.ir}[name], "term", m.MetricsListen)
		live := fresh.Inspect(ctx)
		if !live.ConfigPresent || !live.UnitPresent || !live.MarkerPresent || live.MarkerInstanceID != "term" ||
			!live.MarkerConfigMatches || !live.MarkerUnitMatches || !live.ConfigLoads {
			t.Fatalf("%s: BCC cannot adopt the terminal-created instance: %+v", name, live)
		}
		if _, err := fresh.Retire(ctx, "t1", retireWant(live)); err != nil {
			t.Fatalf("%s retire: %v", name, err)
		}
	}
	// Re-pair the same instance on the same metrics ports after retirement.
	for name, m := range map[string]*Manager{"ex": ex, "ir": ir} {
		_, port := splitPort(m.MetricsListen)
		if _, err := os.Stat(claimPath(m, port)); err == nil {
			t.Fatalf("%s: retire leaked the metrics claim for %s", name, port)
		}
	}
	if _, err := ex.PrepareEX(ctx, "t2", p.exp); err != nil {
		t.Fatalf("re-pair of a retired instance name/port: %v", err)
	}
}

func splitPort(hostport string) (string, string) {
	for i := len(hostport) - 1; i >= 0; i-- {
		if hostport[i] == ':' {
			return hostport[:i], hostport[i+1:]
		}
	}
	return hostport, ""
}
