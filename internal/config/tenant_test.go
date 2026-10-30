package config

import (
	"strings"
	"testing"

	"github.com/zarkmakerburg/baft/internal/recordshape"
	"github.com/zarkmakerburg/baft/internal/securityinternal"
)

func publicKey(t *testing.T) string {
	t.Helper()
	key, err := securityinternal.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := securityinternal.EncodePublicKey(key.Public)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func tenantReadyConfig(t *testing.T) Config {
	t.Helper()
	c := validSharedEX()
	c.Noise = &Noise{
		KeyFile: "/etc/baft/noise-key.json",
		AllowedPeerPublicKeys: map[string]string{
			"urn:baft:node:ir-a": publicKey(t),
			"urn:baft:node:ir-b": publicKey(t),
		},
		RecordShaping: recordshape.Config{},
	}
	if err := Validate(c); err != nil {
		t.Fatalf("fixture config must be valid: %v", err)
	}
	return c
}

func TestAddListenerTenantFailsClosedOnIdentityOrListenerDrift(t *testing.T) {
	base := tenantReadyConfig(t)
	newKey := publicKey(t)
	if _, err := AddListenerTenant(base, "other-ex", "0.0.0.0:8443", "urn:baft:node:ir-c", newKey, "port-443", "127.0.0.1:9003"); err == nil {
		t.Fatal("must refuse a mismatched listener identity")
	}
	if _, err := AddListenerTenant(base, "ex-01", "0.0.0.0:443", "urn:baft:node:ir-c", newKey, "port-443", "127.0.0.1:9003"); err == nil {
		t.Fatal("must refuse a changed listener address")
	}
	if _, err := AddListenerTenant(base, "ex-01", "0.0.0.0:8443", "urn:baft:node:ir-c", newKey, "port-443", "not-an-ip:2444"); err == nil {
		t.Fatal("must refuse a non-IP route target")
	}
}

func TestAddAndRemoveListenerTenantRoundTrip(t *testing.T) {
	base := tenantReadyConfig(t)
	peerID, routeID, target, peerKey := "urn:baft:node:ir-c", "port-443", "127.0.0.1:9003", publicKey(t)
	added, err := AddListenerTenant(base, "ex-01", "0.0.0.0:8443", peerID, peerKey, routeID, target)
	if err != nil {
		t.Fatalf("add tenant: %v", err)
	}
	if err := Validate(added); err != nil {
		t.Fatalf("joined config invalid: %v", err)
	}
	if len(added.Server.AllowedPeerIdentities) != 3 || added.Noise.AllowedPeerPublicKeys[peerID] != peerKey {
		t.Fatal("new tenant identity/key not installed")
	}
	found := false
	for _, route := range added.Routes {
		if route.ID == routeID && route.Target == target && containsTenant(route.AllowedPeers, peerID) {
			found = true
		}
	}
	if !found {
		t.Fatal("tenant-scoped inbound route not installed")
	}
	// The original config must remain unchanged.
	if _, exists := base.Noise.AllowedPeerPublicKeys[peerID]; exists || len(base.Server.AllowedPeerIdentities) != 2 {
		t.Fatal("add mutated the source config")
	}
	repeated, err := AddListenerTenant(added, "ex-01", "0.0.0.0:8443", peerID, peerKey, routeID, target)
	if err != nil {
		t.Fatalf("identical retry must be idempotent: %v", err)
	}
	if len(repeated.Server.AllowedPeerIdentities) != 3 || len(repeated.Routes) != len(added.Routes) {
		t.Fatal("identical retry duplicated tenant state")
	}
	removed, err := RemoveListenerTenant(added, "ex-01", "0.0.0.0:8443", peerID, peerKey, routeID)
	if err != nil {
		t.Fatalf("remove tenant: %v", err)
	}
	if err := Validate(removed); err != nil {
		t.Fatalf("removed config invalid: %v", err)
	}
	if _, exists := removed.Noise.AllowedPeerPublicKeys[peerID]; exists || len(removed.Server.AllowedPeerIdentities) != 2 {
		t.Fatal("tenant identity/key remained after removal")
	}
	for _, route := range removed.Routes {
		if route.ID == routeID && containsTenant(route.AllowedPeers, peerID) {
			t.Fatal("tenant route membership remained after removal")
		}
	}
}

func TestRemoveListenerTenantRefusesFinalTenantAndStaleKey(t *testing.T) {
	base := Config{
		Node: Node{ID: "ex-1", Role: "listener"},
		Server: &Server{Listen: "0.0.0.0:443", AllowedPeerIdentities: []string{"ir-1"}},
		Noise: &Noise{PeerPublicKey: "pinned-key"},
	}
	if _, err := RemoveListenerTenant(base, "ex-1", "0.0.0.0:443", "ir-1", "pinned-key", "service-main"); err == nil || !strings.Contains(err.Error(), "final") {
		t.Fatalf("must refuse removing the final authenticated tenant, got %v", err)
	}
	if _, err := RemoveListenerTenant(base, "ex-1", "0.0.0.0:443", "ir-1", "stale-key", "service-main"); err == nil {
		t.Fatal("must refuse a stale tenant key")
	}
}

func TestTenantMutationClonesMutableConfigFields(t *testing.T) {
	base := Config{
		Node: Node{ID: "ex-1", Role: "listener"},
		Server: &Server{Listen: "0.0.0.0:443", AllowedPeerIdentities: []string{"ir-1"}},
		Noise: &Noise{AllowedPeerPublicKeys: map[string]string{"ir-1": "key-1"}},
		Routes: []Route{{ID: "service-main", Direction: "inbound", Target: "127.0.0.1:2443", AllowedPeers: []string{"ir-1"}}},
	}
	copy := cloneTenantConfig(base)
	copy.Server.AllowedPeerIdentities[0] = "mutated"
	copy.Noise.AllowedPeerPublicKeys["ir-1"] = "mutated"
	copy.Routes[0].AllowedPeers[0] = "mutated"
	if base.Server.AllowedPeerIdentities[0] != "ir-1" ||
		base.Noise.AllowedPeerPublicKeys["ir-1"] != "key-1" ||
		base.Routes[0].AllowedPeers[0] != "ir-1" {
		t.Fatal("tenant transformation clone mutated the source config")
	}
}
