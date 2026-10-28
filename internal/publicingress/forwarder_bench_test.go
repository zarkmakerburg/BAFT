package publicingress

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// Benchmarks for the public-ingress forward() path. The product's Serve path
// fails closed without /proc/net/tcp6, so (like probe_test.go) these drive
// forward() from a bare accept loop. The same harness also fronts the backend
// with a "naive" proxy (dial + two copies, no peek) and with nothing at all
// ("direct"), so the cost of the v0.4.0 peek is isolated by ratio.

func benchListener(tb testing.TB) net.Listener {
	tb.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { ln.Close() })
	return ln
}

// benchBackend echoes; with banner=true it first writes one byte on accept
// (server-speaks-first protocol such as SSH/SMTP).
func benchBackend(tb testing.TB, banner bool) string {
	ln := benchListener(tb)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				if banner {
					if _, err := c.Write([]byte{'B'}); err != nil {
						return
					}
				}
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	return ln.Addr().String()
}

func startNaive(tb testing.TB, target string) string {
	ln := benchListener(tb)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				u, err := net.DialTimeout("tcp4", target, time.Second)
				if err != nil {
					return
				}
				defer u.Close()
				done := make(chan struct{})
				go func() { _, _ = io.Copy(c, u); _ = c.Close(); close(done) }()
				_, _ = io.Copy(u, c)
				_ = u.Close()
				<-done
			}()
		}
	}()
	return ln.Addr().String()
}

func startForward(tb testing.TB, target string) string {
	ln := benchListener(tb)
	f, err := New(Config{Listen: ln.Addr().String(), Target: target, DialTimeout: time.Second})
	if err != nil {
		tb.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func() { defer wg.Done(); f.forward(ctx, c) }()
		}
	}()
	tb.Cleanup(func() { cancel(); ln.Close(); wg.Wait() })
	return ln.Addr().String()
}

func modes(tb testing.TB, backend string) map[string]string {
	return map[string]string{
		"direct":  backend,
		"naive":   startNaive(tb, backend),
		"forward": startForward(tb, backend),
	}
}

var modeOrder = []string{"direct", "naive", "forward"}

func dialClose(b *testing.B, addr string, fn func(c net.Conn)) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c, err := net.Dial("tcp4", addr)
		if err != nil {
			b.Fatal(err)
		}
		_ = c.SetDeadline(time.Now().Add(5 * time.Second))
		fn(c)
		_ = c.Close()
	}
}

// BenchmarkDialThrough measures connect -> first response byte.
//
//	clientfirst: client writes 1 byte immediately, backend echoes it.
//	serverfirst: client is silent, backend sends a banner byte on accept
//	             (v0.4.0 holds the upstream dial for up to the 50 ms peek).
func BenchmarkDialThroughClientFirst(b *testing.B) {
	m := modes(b, benchBackend(b, false))
	for _, name := range modeOrder {
		b.Run(name, func(b *testing.B) {
			one, rb := []byte{'x'}, make([]byte, 1)
			dialClose(b, m[name], func(c net.Conn) {
				if _, err := c.Write(one); err != nil {
					b.Fatal(err)
				}
				if _, err := io.ReadFull(c, rb); err != nil {
					b.Fatal(err)
				}
			})
		})
	}
}

func BenchmarkDialThroughServerFirst(b *testing.B) {
	m := modes(b, benchBackend(b, true))
	for _, name := range modeOrder {
		b.Run(name, func(b *testing.B) {
			rb := make([]byte, 1)
			dialClose(b, m[name], func(c net.Conn) {
				if _, err := io.ReadFull(c, rb); err != nil {
					b.Fatal(err)
				}
			})
		})
	}
}

// BenchmarkDialThroughProbePrefixThenData: the client's first bytes are a valid
// probe prefix but not a full frame, so the peek waits out its whole 50 ms
// window before forwarding (worst case for a client that speaks first).
func BenchmarkDialThroughProbePrefixThenData(b *testing.B) {
	m := modes(b, benchBackend(b, false))
	for _, name := range modeOrder {
		b.Run(name, func(b *testing.B) {
			pre, rb := []byte("BAFT-"), make([]byte, 5)
			dialClose(b, m[name], func(c net.Conn) {
				if _, err := c.Write(pre); err != nil {
					b.Fatal(err)
				}
				if _, err := io.ReadFull(c, rb); err != nil {
					b.Fatal(err)
				}
			})
		})
	}
}

// BenchmarkPassthroughThroughput: one established connection, full duplex
// 256 KiB writes against an echo backend; reports MB/s of one direction.
func BenchmarkPassthroughThroughput(b *testing.B) {
	m := modes(b, benchBackend(b, false))
	const chunk = 256 << 10
	for _, name := range modeOrder {
		b.Run(name, func(b *testing.B) {
			c, err := net.Dial("tcp4", m[name])
			if err != nil {
				b.Fatal(err)
			}
			defer c.Close()
			// Establish (and for forward, pass the peek) before timing.
			_, _ = c.Write([]byte{'x'})
			_, _ = io.ReadFull(c, make([]byte, 1))
			total := int64(b.N) * chunk
			done := make(chan error, 1)
			go func() { _, err := io.CopyN(io.Discard, c, total); done <- err }()
			buf := make([]byte, chunk)
			b.SetBytes(chunk)
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
		})
	}
}

// BenchmarkPassthroughPingPong64B: established-connection small-message RTT.
func BenchmarkPassthroughPingPong64B(b *testing.B) {
	m := modes(b, benchBackend(b, false))
	for _, name := range modeOrder {
		b.Run(name, func(b *testing.B) {
			c, err := net.Dial("tcp4", m[name])
			if err != nil {
				b.Fatal(err)
			}
			defer c.Close()
			_, _ = c.Write([]byte{'x'})
			_, _ = io.ReadFull(c, make([]byte, 1))
			msg, rb := make([]byte, 64), make([]byte, 64)
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
		})
	}
}
