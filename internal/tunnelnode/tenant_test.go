package tunnelnode

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/zarkmakerburg/baft/internal/config"
	"github.com/zarkmakerburg/baft/internal/securityinternal"
)

func TestApplyTenantMutationAddsAndRemovesWithDigestFence(t *testing.T) {
	p := newPair(t)
	p.build(t, "tenant-base")
	for _, n := range []*node{p.ex, p.ir} {
		if _, err := n.Finalize(context.Background(), "tenant-base"); err != nil {
			t.Fatal(err)
		}
	}

	key, err := securityinternal.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := securityinternal.EncodePublicKey(key.Public)
	if err != nil {
		t.Fatal(err)
	}
	readProof := func() (config.Config, string, string, string) {
		t.Helper()
		cfg, err := config.LoadFile(p.ex.liveConfig())
		if err != nil {
			t.Fatal(err)
		}
		cfgBytes, err := os.ReadFile(p.ex.liveConfig())
		if err != nil {
			t.Fatal(err)
		}
		markerBytes, err := os.ReadFile(p.ex.markerPath())
		if err != nil {
			t.Fatal(err)
		}
		unitBytes, err := os.ReadFile(p.ex.unitPath())
		if err != nil {
			t.Fatal(err)
		}
		return cfg, shaHex(cfgBytes), shaHex(unitBytes), shaHex(markerBytes)
	}
	cfg, cfgSHA, unitSHA, markerSHA := readProof()
	if cfg.Server == nil || cfg.Noise == nil {
		t.Fatal("expected a committed EX listener")
	}
	peerID := "urn:baft:node:tenant-test"
	add := TenantMutation{
		ID: "tenant-add-1", Operation: "add", NodeID: cfg.Node.ID, Listen: cfg.Server.Listen,
		ExpectedConfigSHA256: cfgSHA, ExpectedUnitSHA256: unitSHA, ExpectedMarkerSHA256: markerSHA,
		PeerID: peerID, PeerPublicKey: publicKey, RouteID: p.exp.RouteID, Target: "127.0.0.1:2444",
	}
	ev, err := p.ex.ApplyTenantMutation(context.Background(), add)
	if err != nil {
		t.Fatalf("add tenant: %v", err)
	}
	retry, err := p.ex.ApplyTenantMutation(context.Background(), add)
	if err != nil || retry != ev {
		t.Fatalf("identical retry should return durable receipt: evidence=%+v err=%v", retry, err)
	}
	cfg, cfgSHA, unitSHA, markerSHA = readProof()
	if cfg.Noise.AllowedPeerPublicKeys[peerID] != publicKey {
		t.Fatal("tenant key not present in live config")
	}
	remove := TenantMutation{
		ID: "tenant-remove-1", Operation: "remove", NodeID: cfg.Node.ID, Listen: cfg.Server.Listen,
		ExpectedConfigSHA256: cfgSHA, ExpectedUnitSHA256: unitSHA, ExpectedMarkerSHA256: markerSHA,
		PeerID: peerID, PeerPublicKey: publicKey, RouteID: p.exp.RouteID,
	}
	if _, err := p.ex.ApplyTenantMutation(context.Background(), remove); err != nil {
		t.Fatalf("remove tenant: %v", err)
	}
	cfg, _, _, _ = readProof()
	if _, exists := cfg.Noise.AllowedPeerPublicKeys[peerID]; exists {
		t.Fatal("tenant key remained after removal")
	}
}

func TestApplyTenantMutationRejectsStaleDigest(t *testing.T) {
	p := newPair(t)
	p.build(t, "tenant-stale")
	for _, n := range []*node{p.ex, p.ir} {
		if _, err := n.Finalize(context.Background(), "tenant-stale"); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := config.LoadFile(p.ex.liveConfig())
	if err != nil {
		t.Fatal(err)
	}
	markerBytes, err := os.ReadFile(p.ex.markerPath())
	if err != nil {
		t.Fatal(err)
	}
	var marker Marker
	if err := json.Unmarshal(markerBytes, &marker); err != nil {
		t.Fatal(err)
	}
	key, err := securityinternal.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := securityinternal.EncodePublicKey(key.Public)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.ex.ApplyTenantMutation(context.Background(), TenantMutation{
		ID: "tenant-stale-1", Operation: "add", NodeID: cfg.Node.ID, Listen: cfg.Server.Listen,
		ExpectedConfigSHA256: "0000000000000000000000000000000000000000000000000000000000000000",
		ExpectedUnitSHA256: shaHex(mustReadTestFile(t, p.ex.unitPath())), ExpectedMarkerSHA256: shaHex(markerBytes), PeerID: "urn:baft:node:tenant-stale",
		PeerPublicKey: publicKey, RouteID: p.exp.RouteID, Target: "127.0.0.1:2444",
	})
	if err == nil {
		t.Fatal("stale config digest must be rejected")
	}
}

func mustReadTestFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil { t.Fatal(err) }
	return b
}
