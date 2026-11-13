package identity

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"math/big"
	"net"
	"net/url"
	"testing"
	"time"
)

type testPKI struct {
	pool   *x509.CertPool
	server tls.Certificate
	client tls.Certificate
	leaf   *x509.Certificate
}

func newTestPKI(t *testing.T) testPKI {
	t.Helper()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "ca"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, caTpl, caTpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, _ := x509.ParseCertificate(caDER)
	issue := func(serial int64, name string, usage x509.ExtKeyUsage) (tls.Certificate, *x509.Certificate) {
		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		u, _ := url.Parse("urn:baft:node:" + name)
		tpl := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: name}, DNSNames: []string{"srv.test"}, URIs: []*url.URL{u}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), ExtKeyUsage: []x509.ExtKeyUsage{usage}, KeyUsage: x509.KeyUsageDigitalSignature}
		der, err := x509.CreateCertificate(rand.Reader, tpl, ca, &key.PublicKey, caKey)
		if err != nil {
			t.Fatal(err)
		}
		leaf, _ := x509.ParseCertificate(der)
		return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, leaf
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	srv, leaf := issue(0x1234, "srv", x509.ExtKeyUsageServerAuth)
	cli, _ := issue(0x77, "cli", x509.ExtKeyUsageClientAuth)
	return testPKI{pool: pool, server: srv, client: cli, leaf: leaf}
}

type fakeRevocations struct{ ids, serials, fps map[string]bool }

func (f fakeRevocations) IsRevoked(id, serial, fp string) bool {
	return f.ids[id] || f.serials[serial] || f.fps[fp]
}

// handshake runs a real TLS 1.3 handshake and returns the client-side error.
func handshake(t *testing.T, p testPKI, mutate func(*tls.Config)) error {
	t.Helper()
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{p.server}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: p.pool})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.(*tls.Conn).Handshake()
		buf := make([]byte, 1)
		_, _ = c.Read(buf)
	}()
	cfg, err := ClientTLS(p.pool, p.client, "srv.test")
	if err != nil {
		t.Fatal(err)
	}
	mutate(cfg)
	conn, err := net.DialTimeout("tcp", ln.Addr().String(), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	c := tls.Client(conn, cfg)
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if err := c.Handshake(); err != nil {
		return err
	}
	// TLS 1.3 reports a server-side client-cert rejection on first read, not here.
	return nil
}

func TestDialerAcceptsCleanServerAndMatchingPin(t *testing.T) {
	p := newTestPKI(t)
	sum := sha256.Sum256(p.leaf.Raw)
	if err := handshake(t, p, func(c *tls.Config) {
		if err := ApplyServerChecks(c, fakeRevocations{}, hex.EncodeToString(sum[:])); err != nil {
			t.Fatal(err)
		}
	}); err != nil {
		t.Fatalf("clean server with correct pin refused: %v", err)
	}
}

func TestDialerRefusesRevokedServer(t *testing.T) {
	p := newTestPKI(t)
	sum := sha256.Sum256(p.leaf.Raw)
	for name, rev := range map[string]fakeRevocations{
		"identity":    {ids: map[string]bool{"urn:baft:node:srv": true}},
		"serial":      {serials: map[string]bool{"1234": true}},
		"fingerprint": {fps: map[string]bool{hex.EncodeToString(sum[:]): true}},
	} {
		rev := rev
		if err := handshake(t, p, func(c *tls.Config) { _ = ApplyServerChecks(c, rev, "") }); err == nil {
			t.Fatalf("revoked server accepted by %s", name)
		}
	}
}

func TestDialerRefusesWrongPin(t *testing.T) {
	p := newTestPKI(t)
	wrong := sha256.Sum256([]byte("another certificate"))
	if err := handshake(t, p, func(c *tls.Config) { _ = ApplyServerChecks(c, nil, hex.EncodeToString(wrong[:])) }); err == nil {
		t.Fatal("server with a different certificate than the pin was accepted")
	}
}

func TestApplyServerChecksRejectsMalformedPin(t *testing.T) {
	if err := ApplyServerChecks(&tls.Config{}, nil, "abcd"); err == nil {
		t.Fatal("short pin accepted")
	}
}
