package node

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/config"
)

func trustingTLS(t *testing.T, caFile string) *tls.Config {
	t.Helper()
	b, err := os.ReadFile(caFile)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(b) {
		t.Fatal("bad CA")
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, ServerName: "example.com", RootCAs: pool, NextProtos: []string{"h2"}}
}

func TestInitialTransportAlternatesOnlyWithConfiguredFallback(t *testing.T) {
	plain := config.Config{Transport: config.Transport{Primary: "h2"}}
	for n := 0; n < 5; n++ {
		if got := initialTransport(plain, n); got != "h2" {
			t.Fatalf("no fallback configured: attempt %d used %q", n, got)
		}
	}
	fb := config.Config{Transport: config.Transport{Primary: "h2", Fallback: "ws"}}
	want := []string{"h2", "ws", "h2", "ws"}
	for n, w := range want {
		if got := initialTransport(fb, n); got != w {
			t.Fatalf("attempt %d got %q want %q", n, got, w)
		}
	}
}

// A blocked/failing primary carrier at first connect moves to the configured,
// authenticated fallback carrier instead of failing the whole service start.
func TestInitialDialFallsBackWhenPrimaryBlocked(t *testing.T) {
	addr, ca := startH2Server(t)
	// Primary ws cannot open here (no Noise material): it is "blocked". The
	// working carrier is the configured fallback h2 against the live server.
	cfg := config.Config{
		Transport: config.Transport{Primary: "ws", Fallback: "h2"},
		Peer:      &config.Peer{Address: addr, ServerName: "example.com"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	o, err := NewRuntime().dialInitialCarrier(ctx, cfg, trustingTLS(t, ca), 5*time.Second)
	if err != nil {
		t.Fatalf("initial dial did not fall back: %v", err)
	}
	defer o.close()
	if o.transport != "h2" {
		t.Fatalf("connected over %q, want fallback h2", o.transport)
	}
}

// Without a configured fallback a blocked primary is an explicit failure: no
// other (or weaker) carrier is ever tried.
func TestInitialDialWithoutFallbackFailsExplicitly(t *testing.T) {
	addr, ca := startH2Server(t)
	cfg := config.Config{
		Transport: config.Transport{Primary: "ws"},
		Peer:      &config.Peer{Address: addr, ServerName: "example.com"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	o, err := NewRuntime().dialInitialCarrier(ctx, cfg, trustingTLS(t, ca), 300*time.Millisecond)
	if err == nil {
		o.close()
		t.Fatalf("blocked primary succeeded over %q without a configured fallback", o.transport)
	}
	if !strings.Contains(err.Error(), "ws carrier requires noise") {
		t.Fatalf("refusal not explicit: %v", err)
	}
}

// When every candidate is blocked the refusal names each carrier tried.
func TestInitialDialAllCarriersBlockedReportsEach(t *testing.T) {
	cfg := config.Config{
		Transport: config.Transport{Primary: "h2", Fallback: "ws"},
		Peer:      &config.Peer{Address: "127.0.0.1:1", ServerName: "example.com"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	o, err := NewRuntime().dialInitialCarrier(ctx, cfg, &tls.Config{MinVersion: tls.VersionTLS13, ServerName: "example.com", RootCAs: x509.NewCertPool(), NextProtos: []string{"h2"}}, 600*time.Millisecond)
	if err == nil {
		o.close()
		t.Fatal("unreachable peer succeeded")
	}
	if !strings.Contains(err.Error(), "h2:") || !strings.Contains(err.Error(), "ws:") {
		t.Fatalf("error should name both carriers: %v", err)
	}
}
