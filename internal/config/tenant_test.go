package config

import (
	"strings"
	"testing"
)

func TestAddListenerTenantFailsClosedOnIdentityOrListenerDrift(t *testing.T) {
	base := Config{
		Node: Node{ID: "ex-1", Role: "listener"},
		Server: &Server{Listen: "0.0.0.0:443", AllowedPeerIdentities: []string{"ir-1"}},
		Noise: &Noise{PeerPublicKey: "existing-key"},
	}
	if _, err := AddListenerTenant(base, "other-ex", "0.0.0.0:443", "ir-2", "new-key", "service-main", "127.0.0.1:2444"); err == nil {
		t.Fatal("must refuse a mismatched listener identity")
	}
	if _, err := AddListenerTenant(base, "ex-1", "0.0.0.0:8443", "ir-2", "new-key", "service-main", "127.0.0.1:2444"); err == nil {
		t.Fatal("must refuse a changed listener address")
	}
	if _, err := AddListenerTenant(base, "ex-1", "0.0.0.0:443", "ir-2", "new-key", "service-main", "not-an-ip:2444"); err == nil {
		t.Fatal("must refuse a non-IP route target")
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
