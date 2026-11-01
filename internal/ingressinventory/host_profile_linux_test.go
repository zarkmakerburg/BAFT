//go:build linux

package ingressinventory

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// ADR-003: schema 1 is the LAB contract and never carries a profile; schema 2
// is accepted only for host-v1. Every other combination is refused.
func TestInstalledPolicySchemaAndExecutionProfile(t *testing.T) {
	base := installedPolicyV1{NodeID: "ir-01", Platform: "linux", NetNS: "net:[123]", Transport: "dedicated", InventoryV1: ManagedCoverageVersion, Manager: "systemd-socket-v1", SourceRoots: []string{"/etc/systemd/system"}}
	cases := []struct {
		name    string
		raw     string
		profile string
		ok      bool
	}{
		{name: "schema 1 lab", raw: `"schema_version":1`, ok: true},
		{name: "schema 2 host", raw: `"schema_version":2,"execution_profile":"host-v1"`, profile: ExecutionProfileHostV1, ok: true},
		{name: "schema 1 with profile", raw: `"schema_version":1,"execution_profile":"host-v1"`},
		{name: "schema 1 with empty profile key", raw: `"schema_version":1,"execution_profile":""`},
		{name: "schema 2 without profile", raw: `"schema_version":2`},
		{name: "schema 2 unknown profile", raw: `"schema_version":2,"execution_profile":"host-v2"`},
		{name: "schema 3", raw: `"schema_version":3,"execution_profile":"host-v1"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(base)
			if err != nil {
				t.Fatal(err)
			}
			// Replace the zero schema_version the struct emits.
			var m map[string]json.RawMessage
			if err := json.Unmarshal(body, &m); err != nil {
				t.Fatal(err)
			}
			delete(m, "schema_version")
			delete(m, "execution_profile")
			rest, _ := json.Marshal(m)
			raw := append([]byte("{"+tc.raw+","), rest[1:]...)
			got, err := parseInstalledPolicy(raw, base.NodeID, base.NetNS)
			if tc.ok {
				if err != nil || got.ExecutionProfile != tc.profile {
					t.Fatalf("want ok profile=%q, got %+v err=%v (%s)", tc.profile, got, err, raw)
				}
				return
			}
			if !errors.Is(err, ErrUnsupportedManagedNode) {
				t.Fatalf("accepted %s: %v", raw, err)
			}
		})
	}
}

func TestCollectXrayInboundClaimsFiles(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.json")
	bad := filepath.Join(dir, "bad.json")
	missing := filepath.Join(dir, "missing.json")
	sub := filepath.Join(dir, "subdir.json")
	if err := os.WriteFile(good, []byte(`{"inbounds":[{"tag":"vless","port":443},{"tag":"api","listen":"127.0.0.1","port":"62789"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bad, []byte(`{"inbounds":[`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	claims, unknown := collectXrayInboundClaims([]string{missing, good})
	if len(unknown) != 0 || len(claims) != 2 || !claims[0].Covers(443) || !claims[1].Covers(62789) {
		t.Fatalf("claims=%+v unknown=%v", claims, unknown)
	}
	for _, p := range []string{bad, sub} {
		if _, unknown := collectXrayInboundClaims([]string{p}); len(unknown) != 1 {
			t.Fatalf("%s: present but unusable config must be unknown, got %v", p, unknown)
		}
	}
	if claims, unknown := collectXrayInboundClaims([]string{missing}); len(claims)+len(unknown) != 0 {
		t.Fatalf("absent config is neither claim nor unknown: %v %v", claims, unknown)
	}
}
