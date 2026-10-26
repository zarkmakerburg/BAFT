package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

type versionSystem struct {
	mu    sync.Mutex
	out   string
	err   error
	calls int
}

func (v *versionSystem) Systemctl(context.Context, ...string) (string, error) { return "", nil }
func (v *versionSystem) Run(_ context.Context, name string, args ...string) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if filepath.Base(name) == "baft" && strings.Join(args, " ") == "version --json" {
		v.calls++
	}
	return v.out, v.err
}

func capAgent(t *testing.T, sys System) (*Agent, string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "baft")
	if err := os.WriteFile(bin, []byte("v1"), 0o755); err != nil {
		t.Fatal(err)
	}
	return &Agent{cfg: Config{BinDir: dir, System: sys}}, bin
}

func TestCapabilityHeaderCarriesInstalledBinaryTransportCapabilities(t *testing.T) {
	sys := &versionSystem{out: `{"name":"baft","version":"v0.4.0","capabilities":["carrier.h2","carrier.ws","noise.v1","shape.v1","future.cap"]}` + "\n"}
	a, _ := capAgent(t, sys)
	got := a.capabilityHeaders(context.Background())[CapabilityHeader]
	if got != "carrier-h2,carrier-ws,noise-v1,shape-v1" {
		t.Fatalf("header %q: unknown tokens must be dropped and known ones spelled for the header", got)
	}
	tokens, err := parseHeaderForTest(got)
	if err != nil || slices.Contains(tokens, CapabilityOperationalV1) {
		t.Fatalf("operational-ingress-v1 must not appear without a provisioned path: %v %v", tokens, err)
	}
}

func TestCapabilityHeaderReportsNothingFromAnUnreadableBinary(t *testing.T) {
	for name, sys := range map[string]*versionSystem{
		"old binary without the field": {out: `{"name":"baft","version":"0.3.7"}`},
		"exec failure":                 {err: errors.New("exec format error")},
		"not JSON":                     {out: "baft 0.4.0\n"},
	} {
		t.Run(name, func(t *testing.T) {
			a, _ := capAgent(t, sys)
			if h := a.capabilityHeaders(context.Background()); h != nil {
				t.Fatalf("headers %v: nothing may be declared", h)
			}
		})
	}
	a := &Agent{cfg: Config{BinDir: t.TempDir(), System: &versionSystem{out: `{"capabilities":["noise.v1"]}`}}}
	if h := a.capabilityHeaders(context.Background()); h != nil {
		t.Fatalf("missing binary declared %v", h)
	}
}

func TestTransportCapabilitiesAreCachedUntilTheBinaryChanges(t *testing.T) {
	sys := &versionSystem{out: `{"capabilities":["carrier.h2"]}`}
	a, bin := capAgent(t, sys)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_ = a.capabilityHeaders(ctx)
	}
	if sys.calls != 1 {
		t.Fatalf("binary executed %d times for an unchanged binary", sys.calls)
	}
	sys.out = `{"capabilities":["carrier.h2","carrier.ws","noise.v1"]}`
	if err := os.WriteFile(bin, []byte("v2-longer"), 0o755); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Minute)
	_ = os.Chtimes(bin, future, future)
	if got := a.capabilityHeaders(ctx)[CapabilityHeader]; got != "carrier-h2,carrier-ws,noise-v1" || sys.calls != 2 {
		t.Fatalf("after a binary swap: header %q calls %d", got, sys.calls)
	}
}

func parseHeaderForTest(v string) ([]string, error) {
	if v == "" {
		return nil, errors.New("empty")
	}
	return strings.Split(v, ","), nil
}
