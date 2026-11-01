//go:build unix

package ingressauthority

import (
	"strings"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressinventory"
)

// Regression (guided-flow-e2e host leg, run 41281152824): a schema-2 host-v1
// policy was refused at BindInstalledPolicy ("trusted host policy
// unavailable") although the inventory parser accepts it, so every safe host
// mode Apply was blocked before any effect.
func TestInstalledPolicyIdentityAcceptsLABAndHostSchemas(t *testing.T) {
	b := BindingV1{NodeID: "ir-1"}
	base := InstalledPolicyIdentityV1{NodeID: "ir-1", ProfilePath: ingressinventory.InstalledPolicyPath,
		ProfileSHA256: strings.Repeat("a", 64), ProfileDevice: 1, ProfileInode: 2, NetworkNamespace: "net:[4026531840]",
		InventoryVersion: ingressinventory.ManagedCoverageVersion}
	for _, tc := range []struct {
		schema int
		ok     bool
	}{{1, true}, {2, true}, {0, false}, {3, false}} {
		p := base
		p.SchemaVersion = tc.schema
		if got := p.validFor(b); got != tc.ok {
			t.Fatalf("schema %d: validFor=%v want %v", tc.schema, got, tc.ok)
		}
	}
	other := base
	other.SchemaVersion = 2
	if other.validFor(BindingV1{NodeID: "ir-2"}) {
		t.Fatal("identity accepted for another node")
	}
}
