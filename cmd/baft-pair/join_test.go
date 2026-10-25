package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/config"
	"github.com/zarkmakerburg/baft/internal/securityinternal"
)

func joinFixture(t *testing.T) (dir, exPath string, ex securityinternal.KeyPair, c commonFlags, base exPending) {
	t.Helper()
	dir = t.TempDir()
	if err := writePKI(filepath.Join(dir, "pki"), "192.0.2.10", time.Now()); err != nil {
		t.Fatal(err)
	}
	ex, _ = securityinternal.GenerateKeyPair()
	ir1, _ := securityinternal.GenerateKeyPair()
	c = addCommonFlags(flag.NewFlagSet("t", flag.ContinueOnError), filepath.Join(dir, "admin.sock"))
	base = exPending{Version: 1, Identity: "urn:baft:node:ex-1", ServerName: "192.0.2.10"}
	pki := func(n string) string { return filepath.Join(dir, "pki", n) }
	cfg, err := exConfig(base, "urn:baft:node:ir-1", ir1.Public, "0.0.0.0:8443", pki("ca.pem"), pki("server.pem"), pki("server.key"), filepath.Join(dir, "ex.json"), "127.0.0.1:2443", "127.0.0.1", c)
	if err != nil {
		t.Fatal(err)
	}
	exPath = filepath.Join(dir, "ex.yaml")
	if err := writeConfig(exPath, cfg); err != nil {
		t.Fatal(err)
	}
	return
}

func TestJoinAddsTenantToExistingListenerKeepingFirst(t *testing.T) {
	_, exPath, _, c, base := joinFixture(t)
	before, err := config.LoadFile(exPath)
	if err != nil {
		t.Fatal(err)
	}
	ir2, _ := securityinternal.GenerateKeyPair()
	cfg, err := joinConfig(exPath, base, "urn:baft:node:ir-2", ir2.Public, "127.0.0.1:3443", "127.0.0.1", c)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeConfig(exPath, cfg); err != nil {
		t.Fatalf("joined config must validate: %v", err)
	}
	got, err := config.LoadFile(exPath)
	if err != nil {
		t.Fatal(err)
	}
	if got.Server.Listen != before.Server.Listen || got.TLS != before.TLS || got.Node != before.Node {
		t.Fatal("join changed the listener, TLS or node identity")
	}
	if len(got.Server.AllowedPeerIdentities) != 2 || len(got.Noise.AllowedPeerPublicKeys) != 2 || got.Noise.PeerPublicKey != "" {
		t.Fatalf("tenants=%v keys=%d", got.Server.AllowedPeerIdentities, len(got.Noise.AllowedPeerPublicKeys))
	}
	if got.Noise.AllowedPeerPublicKeys["urn:baft:node:ir-1"] != before.Noise.PeerPublicKey {
		t.Fatal("first tenant's pinned key changed")
	}
	if len(got.Routes) != 2 || got.Routes[0].ID != before.Routes[0].ID || got.Routes[0].Target != before.Routes[0].Target {
		t.Fatalf("routes=%+v", got.Routes)
	}
	// Same default route id for both tenants, disjoint allowed peers.
	if got.Routes[0].ID != got.Routes[1].ID || got.Routes[0].Target == got.Routes[1].Target {
		t.Fatalf("tenant routes should share the id but not the target: %+v", got.Routes)
	}
	if got.Routes[0].AllowedPeers[0] != "urn:baft:node:ir-1" || got.Routes[1].AllowedPeers[0] != "urn:baft:node:ir-2" {
		t.Fatal("allowed peers are not tenant scoped")
	}
}

func TestJoinRefusesExistingTenantWrongExAndKeyReuse(t *testing.T) {
	_, exPath, _, c, base := joinFixture(t)
	k, _ := securityinternal.GenerateKeyPair()
	if _, err := joinConfig(exPath, base, "urn:baft:node:ir-1", k.Public, "127.0.0.1:3443", "127.0.0.1", c); err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("existing tenant must not be overwritten, got %v", err)
	}
	other := base
	other.Identity = "urn:baft:node:ex-2"
	if _, err := joinConfig(exPath, other, "urn:baft:node:ir-3", k.Public, "127.0.0.1:3443", "127.0.0.1", c); err == nil {
		t.Fatal("pairing for a different EX must be refused")
	}
	shaped := base
	shaped.RecordShaping = true
	if _, err := joinConfig(exPath, shaped, "urn:baft:node:ir-3", k.Public, "127.0.0.1:3443", "127.0.0.1", c); err == nil {
		t.Fatal("record shaping mismatch must be refused")
	}
	// Reusing tenant 1's Noise key under a new identity must fail validation.
	cur, _ := config.LoadFile(exPath)
	dup, _ := securityinternal.DecodePublicKey(cur.Noise.PeerPublicKey)
	cfg, err := joinConfig(exPath, base, "urn:baft:node:ir-4", dup, "127.0.0.1:3443", "127.0.0.1", c)
	if err == nil {
		if err = writeConfig(filepath.Join(t.TempDir(), "x.yaml"), cfg); err == nil {
			t.Fatal("a Noise key shared by two tenants must be rejected")
		}
	}
	if _, err := os.Stat(exPath); err != nil {
		t.Fatal(err)
	}
}
