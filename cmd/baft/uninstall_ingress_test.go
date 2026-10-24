package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zarkmakerburg/baft/internal/uninstall"
)

func TestRemoveIngressChainScopeAndRefusal(t *testing.T) {
	dir := t.TempDir()
	helper := filepath.Join(dir, "helper.sh")
	log := filepath.Join(dir, "log")
	script := "#!/bin/sh\necho \"$1 $BAFT_INGRESS_CHAIN_MARKER $BAFT_SYSTEMD_DIR\" >> " + log + "\nexit ${HELPER_RC:-0}\n"
	if err := os.WriteFile(helper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	old := uninstallIngressHelper
	uninstallIngressHelper = helper
	t.Cleanup(func() { uninstallIngressHelper = old })
	env := &uninstall.Env{AgentStateDir: "/s", UnitDir: "/u"}
	run := func(o uninstall.Options) string {
		var out bytes.Buffer
		removeIngressChain(context.Background(), env, o, uninstallIO{out: &out})
		return out.String()
	}
	// Out of scope: BCC-only and binaries-only never touch the chain.
	run(uninstall.Options{Scope: uninstall.Scope{BCC: true}})
	run(uninstall.Options{Scope: uninstall.Scope{Binaries: true}})
	if _, err := os.Stat(log); err == nil {
		t.Fatal("helper ran for an out-of-scope uninstall")
	}
	run(uninstall.Options{}) // default scope includes the agent
	b, _ := os.ReadFile(log)
	if strings.TrimSpace(string(b)) != "remove /s/ingress-chain.owner /u" {
		t.Fatalf("helper call: %q", b)
	}
	t.Setenv("HELPER_RC", "3")
	if out := run(uninstall.Options{Scope: uninstall.Scope{Full: true}}); !strings.Contains(out, "left in place") {
		t.Fatalf("refusal not reported: %q", out)
	}
}
