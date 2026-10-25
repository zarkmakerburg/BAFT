// Package benchpath holds release-comparison benchmarks of the BAFT tunnel
// data path. Files that compile against both v0.3.7 and v0.4.0 use only APIs
// present in both; v0.4.0-only knobs live in separate files.
package benchpath

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/routes"
	"github.com/zarkmakerburg/baft/internal/session"
)

func tcpPair(tb testing.TB) (a, b net.Conn) {
	tb.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		tb.Fatal(err)
	}
	defer ln.Close()
	ch := make(chan net.Conn, 1)
	go func() { c, _ := ln.Accept(); ch <- c }()
	a, err = net.Dial("tcp4", ln.Addr().String())
	if err != nil {
		tb.Fatal(err)
	}
	b = <-ch
	return a, b
}

// echoTarget accepts connections and echoes every byte back.
func echoTarget(tb testing.TB) string {
	tb.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	return ln.Addr().String()
}

// tunnel builds a connected Dialer/Listener session pair over a loopback TCP
// carrier (no TLS/Noise; the carrier packages are byte-identical between the
// two releases, so this isolates internal/session). mutate may adjust Options.
func tunnel(tb testing.TB, mutate func(*session.Options)) (*session.Peer, context.Context) {
	tb.Helper()
	target := echoTarget(tb)
	const lPeer = "urn:baft:node:ir-01"
	tbl, err := routes.New([]routes.Route{{ID: "echo", Target: target, AllowedPeers: map[string]struct{}{lPeer: {}}}})
	if err != nil {
		tb.Fatal(err)
	}
	ca, cb := tcpPair(tb)
	dopt := session.Options{NodeID: "ir-01", ExpectedPeerNodeID: "ex-01"}
	lopt := session.Options{NodeID: "ex-01", ExpectedPeerNodeID: "ir-01"}
	if mutate != nil {
		mutate(&dopt)
		mutate(&lopt)
	}
	d, err := session.New(session.Dialer, session.Carrier{In: ca, Out: ca}, "urn:baft:node:ex-01", nil, dopt)
	if err != nil {
		tb.Fatal(err)
	}
	l, err := session.New(session.Listener, session.Carrier{In: cb, Out: cb}, lPeer, tbl, lopt)
	if err != nil {
		tb.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	tb.Cleanup(func() { cancel(); ca.Close(); cb.Close() })
	go func() { _ = d.Run(ctx) }()
	go func() { _ = l.Run(ctx) }()
	// Warm: first flow also waits for the session handshake.
	c := openFlow(tb, ctx, d)
	_, _ = c.Write([]byte{1})
	_, _ = io.ReadFull(c, make([]byte, 1))
	c.Close()
	return d, ctx
}

func openFlow(tb testing.TB, ctx context.Context, d *session.Peer) net.Conn {
	tb.Helper()
	app, inner := tcpPair(tb)
	// The ctx passed to OpenFlow also scopes the flow's pump goroutines, so it
	// must outlive the call (no per-call timeout/cancel).
	if err := d.OpenFlow(ctx, "echo", inner); err != nil {
		tb.Fatal(err)
	}
	_ = app.SetDeadline(time.Now().Add(120 * time.Second)) // fail instead of hanging
	return app
}

func benchOpen(b *testing.B, mutate func(*session.Options)) {
	d, ctx := tunnel(b, mutate)
	one := []byte{7}
	rb := make([]byte, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := openFlow(b, ctx, d)
		if _, err := c.Write(one); err != nil {
			b.Fatal(err)
		}
		if _, err := io.ReadFull(c, rb); err != nil {
			b.Fatal(err)
		}
		c.Close()
	}
}

func benchPingPong(b *testing.B, size int, mutate func(*session.Options)) {
	d, ctx := tunnel(b, mutate)
	c := openFlow(b, ctx, d)
	defer c.Close()
	msg := make([]byte, size)
	rb := make([]byte, size)
	b.SetBytes(int64(size))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.Write(msg); err != nil {
			b.Fatal(err)
		}
		if _, err := io.ReadFull(c, rb); err != nil {
			b.Fatal(err)
		}
	}
}

// benchStream pushes b.N*chunk bytes through one flow while a reader drains
// the echo concurrently (full duplex, window/credit flow control engaged).
func benchStream(b *testing.B, chunk int, mutate func(*session.Options)) {
	d, ctx := tunnel(b, mutate)
	c := openFlow(b, ctx, d)
	defer c.Close()
	total := int64(b.N) * int64(chunk)
	done := make(chan error, 1)
	go func() {
		_, err := io.CopyN(io.Discard, c, total)
		done <- err
	}()
	buf := make([]byte, chunk)
	b.SetBytes(int64(chunk))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.Write(buf); err != nil {
			b.Fatal(err)
		}
	}
	if err := <-done; err != nil {
		b.Fatal(err)
	}
}

func BenchmarkSessionOpenFlowEcho1B(b *testing.B)   { benchOpen(b, nil) }
func BenchmarkSessionPingPong64B(b *testing.B)      { benchPingPong(b, 64, nil) }
func BenchmarkSessionPingPong32KiB(b *testing.B)    { benchPingPong(b, 32<<10, nil) }
func BenchmarkSessionStreamEcho256KiB(b *testing.B) { benchStream(b, 256<<10, nil) }
