package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/config"
	"github.com/zarkmakerburg/baft/internal/securityinternal"
)

// The generated pair must pass the same loader `baft run` uses, and each
// side must name the other's identity and pin the other's key.
func TestGeneratedConfigsLoadAndPinEachOther(t *testing.T) {
	dir := t.TempDir()
	if err := writePKI(filepath.Join(dir, "pki"), "192.0.2.10", time.Now()); err != nil {
		t.Fatal(err)
	}
	ex, _ := securityinternal.GenerateKeyPair()
	ir, _ := securityinternal.GenerateKeyPair()
	ca, _ := os.ReadFile(filepath.Join(dir, "pki", "ca.pem"))
	d, psk, err := securityinternal.NewPairingDescriptor("192.0.2.10:8443", "192.0.2.10", "urn:baft:node:ex-1", ex.Public, ca, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	c := addCommonFlags(flag.NewFlagSet("t", flag.ContinueOnError), filepath.Join(dir, "admin.sock"))

	irCfg, err := irConfig(d, "urn:baft:node:ir-1", filepath.Join(dir, "pki", "ca.pem"), filepath.Join(dir, "ir.json"), "127.0.0.1:1443", "0.0.0.0", c)
	if err != nil {
		t.Fatal(err)
	}
	irPath := filepath.Join(dir, "ir.yaml")
	if err := writeConfig(irPath, irCfg); err != nil {
		t.Fatal(err)
	}
	reply, err := securityinternal.NewPairingReply(d, ex.Public, psk, "urn:baft:node:ir-1", ir.Public)
	if err != nil {
		t.Fatal(err)
	}
	id, pub, err := securityinternal.DecodePairingReply(reply, psk, ex.Public, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	p := exPending{Version: 1, Identity: d.NodeIdentity, ServerName: d.ServerName}
	pki := func(n string) string { return filepath.Join(dir, "pki", n) }
	exCfg, err := exConfig(p, id, pub, "0.0.0.0:8443", pki("ca.pem"), pki("server.pem"), pki("server.key"), filepath.Join(dir, "ex.json"), "127.0.0.1:2443", "127.0.0.1", c)
	if err != nil {
		t.Fatal(err)
	}
	exPath := filepath.Join(dir, "ex.yaml")
	if err := writeConfig(exPath, exCfg); err != nil {
		t.Fatal(err)
	}

	gotIR, err := config.LoadFile(irPath)
	if err != nil {
		t.Fatal(err)
	}
	gotEX, err := config.LoadFile(exPath)
	if err != nil {
		t.Fatal(err)
	}
	exPub, _ := securityinternal.EncodePublicKey(ex.Public)
	irPub, _ := securityinternal.EncodePublicKey(ir.Public)
	if gotIR.Noise.PeerPublicKey != exPub || gotEX.Noise.PeerPublicKey != irPub {
		t.Fatal("configs do not pin each other's Noise key")
	}
	if gotIR.Peer.AllowedIdentity != "urn:baft:node:"+gotEX.Node.ID || gotEX.Server.AllowedPeerIdentities[0] != "urn:baft:node:"+gotIR.Node.ID {
		t.Fatal("node ids do not match the identities each side expects")
	}
	if gotIR.TLS.CertFile != "" || gotIR.TLS.KeyFile != "" {
		t.Fatal("Noise dialer config carries an unused client certificate")
	}
	if gotIR.Routes[0].RemoteRoute != gotEX.Routes[0].ID {
		t.Fatal("route ids do not match")
	}
}

func TestPKIServerCertificateVerifiesForHost(t *testing.T) {
	for _, host := range []string{"192.0.2.10", "ex.example"} {
		dir := t.TempDir()
		if err := writePKI(dir, host, time.Now()); err != nil {
			t.Fatal(err)
		}
		pair, err := tls.LoadX509KeyPair(filepath.Join(dir, "server.pem"), filepath.Join(dir, "server.key"))
		if err != nil {
			t.Fatal(err)
		}
		caPEM, _ := os.ReadFile(filepath.Join(dir, "ca.pem"))
		roots := x509.NewCertPool()
		roots.AppendCertsFromPEM(caPEM)
		leaf, _ := x509.ParseCertificate(pair.Certificate[0])
		if _, err := leaf.Verify(x509.VerifyOptions{DNSName: host, Roots: roots}); err != nil {
			t.Fatalf("%s: %v", host, err)
		}
		for _, n := range []string{"ca.key", "server.key"} {
			st, _ := os.Stat(filepath.Join(dir, n))
			if st.Mode().Perm() != 0o600 {
				t.Fatalf("%s mode %v", n, st.Mode().Perm())
			}
		}
	}
}

// With carried ports, the IR serves each port publicly and the EX exits each
// route to the same port, and both configs still load.
func TestCarriedPortsBuildMatchingPublicRoutes(t *testing.T) {
	dir := t.TempDir()
	if err := writePKI(filepath.Join(dir, "pki"), "192.0.2.10", time.Now()); err != nil {
		t.Fatal(err)
	}
	ex, _ := securityinternal.GenerateKeyPair()
	ir, _ := securityinternal.GenerateKeyPair()
	ca, _ := os.ReadFile(filepath.Join(dir, "pki", "ca.pem"))
	d, psk, err := securityinternal.NewPairingDescriptor("192.0.2.10:8443", "192.0.2.10", "urn:baft:node:ex-1", ex.Public, ca, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ports, err := parsePorts(" 443, 2053 ")
	if err != nil {
		t.Fatal(err)
	}
	d.Ports = ports
	code, err := d.Encode()
	if err != nil {
		t.Fatal(err)
	}
	d, _, _, _, err = securityinternal.DecodePairingDescriptor(code, time.Now())
	if err != nil || len(d.Ports) != 2 {
		t.Fatalf("ports did not survive the pairing code: %v %v", d.Ports, err)
	}
	c := addCommonFlags(flag.NewFlagSet("t", flag.ContinueOnError), filepath.Join(dir, "admin.sock"))
	irCfg, err := irConfig(d, "urn:baft:node:ir-1", filepath.Join(dir, "pki", "ca.pem"), filepath.Join(dir, "ir.json"), "127.0.0.1:1443", "0.0.0.0", c)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeConfig(filepath.Join(dir, "ir.yaml"), irCfg); err != nil {
		t.Fatal(err)
	}
	reply, _ := securityinternal.NewPairingReply(d, ex.Public, psk, "urn:baft:node:ir-1", ir.Public)
	id, pub, err := securityinternal.DecodePairingReply(reply, psk, ex.Public, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	p := exPending{Version: 1, Identity: d.NodeIdentity, ServerName: d.ServerName, Ports: ports}
	pki := func(n string) string { return filepath.Join(dir, "pki", n) }
	exCfg, err := exConfig(p, id, pub, "0.0.0.0:8443", pki("ca.pem"), pki("server.pem"), pki("server.key"), filepath.Join(dir, "ex.json"), "127.0.0.1:2443", "127.0.0.1", c)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeConfig(filepath.Join(dir, "ex.yaml"), exCfg); err != nil {
		t.Fatal(err)
	}
	if len(irCfg.Routes) != 2 || len(exCfg.Routes) != 2 {
		t.Fatalf("want 2 routes each, got IR %d EX %d", len(irCfg.Routes), len(exCfg.Routes))
	}
	for i, want := range []string{"443", "2053"} {
		r, e := irCfg.Routes[i], exCfg.Routes[i]
		if !r.Public || r.Listen != "0.0.0.0:"+want || r.RemoteRoute != e.ID || e.Target != "127.0.0.1:"+want {
			t.Fatalf("route %d mismatch: IR %+v EX %+v", i, r, e)
		}
	}
	for _, bad := range []string{"0", "70000", "443,443", "x"} {
		if _, err := parsePorts(bad); err == nil {
			t.Fatalf("parsePorts(%q) accepted", bad)
		}
	}
}


func TestJoinConfigPreservesOwnerAndAddsOnlyNewTenant(t *testing.T) {
	dir := t.TempDir()
	pkiDir := filepath.Join(dir, "pki")
	if err := writePKI(pkiDir, "192.0.2.10", time.Now()); err != nil {
		t.Fatal(err)
	}
	ex, err := securityinternal.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	ownerIR, err := securityinternal.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	newIR, err := securityinternal.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	ca, err := os.ReadFile(filepath.Join(pkiDir, "ca.pem"))
	if err != nil {
		t.Fatal(err)
	}
	exIdentity := "urn:baft:node:ex-1"
	ownerIdentity, newIdentity := "urn:baft:node:ir-1", "urn:baft:node:ir-2"
	d, _, err := securityinternal.NewPairingDescriptor("192.0.2.10:8443", "192.0.2.10", exIdentity, ex.Public, ca, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	p := exPending{Version: 1, Identity: d.NodeIdentity, ServerName: d.ServerName}
	pki := func(name string) string { return filepath.Join(pkiDir, name) }
	ownerFlags := addCommonFlags(flag.NewFlagSet("owner", flag.ContinueOnError), filepath.Join(dir, "admin.sock"))
	ownerCfg, err := exConfig(p, ownerIdentity, ownerIR.Public, "0.0.0.0:8443", pki("ca.pem"), pki("server.pem"), pki("server.key"), filepath.Join(dir, "ex.json"), "127.0.0.1:2443", "127.0.0.1", ownerFlags)
	if err != nil {
		t.Fatal(err)
	}
	ownerPath := filepath.Join(dir, "owner.yaml")
	if err := writeConfig(ownerPath, ownerCfg); err != nil {
		t.Fatal(err)
	}

	joinFlags := addCommonFlags(flag.NewFlagSet("join", flag.ContinueOnError), filepath.Join(dir, "admin.sock"))
	*joinFlags.routeID = "tenant-ir-2"
	joined, err := joinConfig(ownerPath, p, newIdentity, newIR.Public, "127.0.0.1:2444", "127.0.0.1", joinFlags)
	if err != nil {
		t.Fatal(err)
	}
	if joined.Server.Listen != ownerCfg.Server.Listen || joined.Server.ServerName != ownerCfg.Server.ServerName {
		t.Fatalf("joining changed listener identity: before=%+v after=%+v", ownerCfg.Server, joined.Server)
	}
	if len(joined.Server.AllowedPeerIdentities) != 2 ||
		joined.Server.AllowedPeerIdentities[0] != ownerIdentity ||
		joined.Server.AllowedPeerIdentities[1] != newIdentity {
		t.Fatalf("existing and new tenants were not both retained: %v", joined.Server.AllowedPeerIdentities)
	}
	ownerPub, err := securityinternal.EncodePublicKey(ownerIR.Public)
	if err != nil {
		t.Fatal(err)
	}
	newPub, err := securityinternal.EncodePublicKey(newIR.Public)
	if err != nil {
		t.Fatal(err)
	}
	if joined.Noise.AllowedPeerPublicKeys[ownerIdentity] != ownerPub ||
		joined.Noise.AllowedPeerPublicKeys[newIdentity] != newPub {
		t.Fatalf("Noise peer keys were not preserved and extended: %v", joined.Noise.AllowedPeerPublicKeys)
	}
	if len(joined.Routes) != 2 {
		t.Fatalf("want original and new tenant routes, got %+v", joined.Routes)
	}
	if got, want := joined.Routes[0], ownerCfg.Routes[0]; got.ID != want.ID || got.Target != want.Target ||
		len(got.AllowedPeers) != 1 || got.AllowedPeers[0] != ownerIdentity {
		t.Fatalf("joining changed the owner's route: before=%+v after=%+v", want, got)
	}
	if got := joined.Routes[1]; got.ID != "tenant-ir-2" || got.Target != "127.0.0.1:2444" ||
		len(got.AllowedPeers) != 1 || got.AllowedPeers[0] != newIdentity {
		t.Fatalf("new tenant route mismatch: %+v", got)
	}

	joinedPath := filepath.Join(dir, "joined.yaml")
	if err := writeConfig(joinedPath, joined); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.LoadFile(joinedPath)
	if err != nil {
		t.Fatalf("joined config does not load: %v", err)
	}
	if len(loaded.Server.AllowedPeerIdentities) != 2 || len(loaded.Routes) != 2 {
		t.Fatalf("validated config lost tenant state: %+v", loaded)
	}
	if _, err := joinConfig(joinedPath, p, newIdentity, newIR.Public, "127.0.0.1:2444", "127.0.0.1", joinFlags); err == nil {
		t.Fatal("duplicate tenant join was accepted")
	}
}
