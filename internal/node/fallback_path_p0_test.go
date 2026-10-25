package node

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/config"
)

func startH2Server(t *testing.T) (addr string, caPEM string) {
	t.Helper()
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_ = http.NewResponseController(w).Flush()
		<-r.Context().Done()
	}))
	ts.EnableHTTP2 = true
	ts.StartTLS()
	t.Cleanup(ts.Close)
	path := filepath.Join(t.TempDir(), "ca.pem")
	block := &pem.Block{Type: "CERTIFICATE", Bytes: ts.Certificate().Raw}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	return strings.TrimPrefix(ts.URL, "https://"), path
}

func unrelatedCAFile(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: "unrelated-ca"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "other-ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// primary-only TLS state, as runDialer builds it: wrong name, empty trust.
func primaryOnlyTLS() *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS13, ServerName: "primary.invalid", RootCAs: x509.NewCertPool(), NextProtos: []string{"h2"}}
}

// peer.fallback_address / fallback_server_name / fallback_ca_file are accepted
// by config.Validate, so the runtime must really honour them for the fallback
// carrier (and only for it) instead of silently dialling the primary path.
func TestFallbackCarrierUsesDistinctPeerPath(t *testing.T) {
	fbAddr, fbCA := startH2Server(t)
	cfg := config.Config{
		Transport: config.Transport{Primary: "ws", Fallback: "h2"},
		Peer: &config.Peer{
			Address: "127.0.0.1:1", ServerName: "primary.invalid",
			FallbackAddress: fbAddr, FallbackServerName: "example.com", FallbackCAFile: fbCA,
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	o, err := NewRuntime().dialCarrierTransport(ctx, cfg, primaryOnlyTLS(), "h2")
	if err != nil {
		t.Fatalf("fallback carrier did not use peer.fallback_* path: %v", err)
	}
	o.close()
}

// The fallback never verifies against the primary CA when it names its own,
// and an unreadable/invalid fallback CA is an explicit failure, not a quiet
// reuse of the primary trust.
func TestFallbackCarrierNeverLoosensOrSilentlyReusesTrust(t *testing.T) {
	fbAddr, _ := startH2Server(t)
	otherCA := unrelatedCAFile(t) // a trusted CA that did not sign the fallback server
	base := config.Config{
		Transport: config.Transport{Primary: "ws", Fallback: "h2"},
		Peer:      &config.Peer{Address: "127.0.0.1:1", ServerName: "primary.invalid", FallbackAddress: fbAddr, FallbackServerName: "example.com"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	wrong := base
	peer := *base.Peer
	peer.FallbackCAFile = otherCA
	wrong.Peer = &peer
	if o, err := NewRuntime().dialCarrierTransport(ctx, wrong, primaryOnlyTLS(), "h2"); err == nil {
		o.close()
		t.Fatal("fallback carrier accepted a server not signed by its fallback CA")
	}

	missing := base
	peer2 := *base.Peer
	peer2.FallbackCAFile = filepath.Join(t.TempDir(), "absent.pem")
	missing.Peer = &peer2
	if o, err := NewRuntime().dialCarrierTransport(ctx, missing, primaryOnlyTLS(), "h2"); err == nil {
		o.close()
		t.Fatal("unreadable fallback CA was ignored")
	} else if !strings.Contains(err.Error(), "fallback carrier CA") {
		t.Fatalf("unreadable fallback CA error not explicit: %v", err)
	}
}

// The primary carrier keeps using the primary peer path even when a distinct
// fallback path is configured.
func TestPrimaryCarrierIgnoresFallbackPeerPath(t *testing.T) {
	fbAddr, fbCA := startH2Server(t)
	cfg := config.Config{
		Transport: config.Transport{Primary: "h2", Fallback: "ws"},
		Peer: &config.Peer{
			Address: "127.0.0.1:1", ServerName: "primary.invalid",
			FallbackAddress: fbAddr, FallbackServerName: "example.com", FallbackCAFile: fbCA,
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if o, err := NewRuntime().dialCarrierTransport(ctx, cfg, primaryOnlyTLS(), "h2"); err == nil {
		o.close()
		t.Fatal("primary carrier reached the fallback server; it must dial peer.address only")
	}
}
