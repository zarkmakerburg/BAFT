package publicingress

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const testProbe = "BAFT-INGRESS-PROBE/1 0123456789abcdef0123456789abcdef\n"

// recordingBackend counts accepts and records every byte received per conn.
type recordingBackend struct {
	ln      net.Listener
	accepts atomic.Int64
	mu      sync.Mutex
	got     [][]byte
	banner  string
}

func newRecordingBackend(t *testing.T, banner string) *recordingBackend {
	t.Helper()
	b := &recordingBackend{ln: tcpListener(t), banner: banner}
	go func() {
		for {
			c, err := b.ln.Accept()
			if err != nil {
				return
			}
			b.accepts.Add(1)
			go func() {
				defer c.Close()
				if b.banner != "" {
					_, _ = io.WriteString(c, b.banner)
				}
				data, _ := io.ReadAll(c)
				b.mu.Lock()
				b.got = append(b.got, data)
				b.mu.Unlock()
			}()
		}
	}()
	return b
}

func (b *recordingBackend) received() [][]byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([][]byte(nil), b.got...)
}

func waitReceived(b *recordingBackend) [][]byte {
	deadline := time.Now().Add(2 * time.Second)
	for len(b.received()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	return b.received()
}

// launcher starts a forwarder in front of target. "serve" uses the production
// Serve path (listener admission, MaxClients, live-port inventory); "direct"
// drives the same per-connection forward() from a bare accept loop so the
// probe/peek behavior stays verifiable where /proc/net/tcp6 is unavailable and
// Serve fails closed.
type launcher struct {
	name string
	run  func(t *testing.T, target string, limit int) (*Forwarder, string, func())
}

func launchDirect(t *testing.T, target string, limit int) (*Forwarder, string, func()) {
	t.Helper()
	ln := tcpListener(t)
	f, err := New(Config{Listen: ln.Addr().String(), Target: target, MaxClients: limit, DialTimeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func() { defer wg.Done(); f.forward(ctx, c) }()
		}
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			_ = ln.Close()
			done := make(chan struct{})
			go func() { <-acceptDone; wg.Wait(); close(done) }()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Error("direct forwarder goroutines leaked")
			}
		})
	}
	t.Cleanup(stop)
	return f, ln.Addr().String(), stop
}

func eachLauncher(t *testing.T, fn func(*testing.T, launcher)) {
	for _, l := range []launcher{{"serve", launch}, {"direct", launchDirect}} {
		t.Run(l.name, func(t *testing.T) { fn(t, l) })
	}
}

func TestProbeEchoesWithoutDialingTarget(t *testing.T) { eachLauncher(t, testProbeEchoes) }

func testProbeEchoes(t *testing.T, l launcher) {
	launch := l.run
	backend := newRecordingBackend(t, "")
	f, address, _ := launch(t, backend.ln.Addr().String(), 1)
	c := connect(t, address)
	if _, err := io.WriteString(c, testProbe); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(testProbe))
	if _, err := io.ReadFull(c, got); err != nil || string(got) != testProbe {
		t.Fatalf("echo %q %v", got, err)
	}
	if n, err := c.Read(make([]byte, 1)); err == nil || n != 0 {
		t.Fatalf("connection not closed after echo: %d %v", n, err)
	}
	time.Sleep(100 * time.Millisecond)
	if backend.accepts.Load() != 0 {
		t.Fatal("probe dialed the target")
	}
	deadline := time.Now().Add(time.Second)
	for f.ActiveConnections() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if f.ActiveConnections() != 0 || f.RejectedConnections() != 0 {
		t.Fatalf("accounting %d/%d", f.ActiveConnections(), f.RejectedConnections())
	}
}

func TestProbeValidatorIsStrict(t *testing.T) {
	good := []byte(testProbe)
	if !IsProbeFrame(good) {
		t.Fatal("valid frame rejected")
	}
	bad := [][]byte{
		nil, good[:len(good)-1], append(append([]byte(nil), good...), '\n'),
		append(append([]byte(nil), good...), 'x'),
		bytes.Replace(good, []byte("0123"), []byte("0A23"), 1),
		bytes.Replace(good, []byte("0123"), []byte("012g"), 1),
		bytes.Replace(good, []byte("/1"), []byte("/2"), 1),
		append(append([]byte(nil), good[:len(good)-1]...), '\r'),
		bytes.Replace(good, []byte("1 0"), []byte("1  "), 1),
	}
	for i, b := range bad {
		if IsProbeFrame(b) {
			t.Errorf("case %d accepted: %q", i, b)
		}
	}
}

func TestForwarderPassesThroughMalformedProbeByteExact(t *testing.T) {
	eachLauncher(t, testMalformed)
}

func testMalformed(t *testing.T, l launcher) {
	launch := l.run
	cases := map[string]string{
		"partial":        testProbe[:30],
		"uppercase":      "BAFT-INGRESS-PROBE/1 0123456789ABCDEF0123456789abcdef\n",
		"short-nonce":    "BAFT-INGRESS-PROBE/1 0123\n",
		"crlf":           testProbe[:len(testProbe)-1] + "\r\n",
		"trailing-bytes": testProbe + "GET / HTTP/1.0\r\n\r\n",
		"oversized":      testProbe + string(bytes.Repeat([]byte{'z'}, 200)),
		"prefix-only":    "BAFT-INGRESS-PROBE/1 ",
		"ordinary":       "hello world",
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			backend := newRecordingBackend(t, "")
			_, address, _ := launch(t, backend.ln.Addr().String(), 2)
			c := connect(t, address)
			if _, err := io.WriteString(c, payload); err != nil {
				t.Fatal(err)
			}
			_ = c.CloseWrite()
			if reply, _ := io.ReadAll(c); len(reply) != 0 {
				t.Fatalf("unexpected reply %q", reply)
			}
			r := waitReceived(backend)
			if len(r) != 1 || string(r[0]) != payload {
				t.Fatalf("backend got %q want %q", r, payload)
			}
		})
	}
}

func TestForwarderPassesThroughSplitBytesInOrder(t *testing.T) { eachLauncher(t, testSplit) }

func testSplit(t *testing.T, l launcher) {
	launch := l.run
	backend := newRecordingBackend(t, "")
	_, address, _ := launch(t, backend.ln.Addr().String(), 2)
	c := connect(t, address)
	// Probe-prefix bytes arrive slowly, then diverge; all must arrive in order.
	for _, p := range []string{"BAFT-INGRESS-", "PROBE/1 0123", "XYZ", "-tail"} {
		if _, err := io.WriteString(c, p); err != nil {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = c.CloseWrite()
	_, _ = io.ReadAll(c)
	want := "BAFT-INGRESS-PROBE/1 0123XYZ-tail"
	if r := waitReceived(backend); len(r) != 1 || string(r[0]) != want {
		t.Fatalf("got %q want %q", r, want)
	}
}

func TestForwarderSilentClientStillConnects(t *testing.T) { eachLauncher(t, testSilent) }

func testSilent(t *testing.T, l launcher) {
	launch := l.run
	backend := echoBackend(t)
	_, address, _ := launch(t, backend.Addr().String(), 2)
	c := connect(t, address)
	time.Sleep(150 * time.Millisecond) // well past the 50ms peek window
	if _, err := io.WriteString(c, "late"); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 4)
	if _, err := io.ReadFull(c, got); err != nil || string(got) != "late" {
		t.Fatalf("late write lost: %q %v", got, err)
	}
}

func TestForwarderServerSpeaksFirst(t *testing.T) { eachLauncher(t, testServerFirst) }

func testServerFirst(t *testing.T, l launcher) {
	launch := l.run
	const banner = "220 hello\r\n"
	backend := newRecordingBackend(t, banner)
	_, address, _ := launch(t, backend.ln.Addr().String(), 2)
	c := connect(t, address)
	got := make([]byte, len(banner))
	start := time.Now()
	if _, err := io.ReadFull(c, got); err != nil || string(got) != banner {
		t.Fatalf("banner %q %v", got, err)
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("banner delayed %v", d)
	}
}

func TestProbeRespectsMaxClientsAndReleasesSlot(t *testing.T) {
	// MaxClients admission lives in Serve, so only the production path applies.
	testMaxClients(t)
}

func testMaxClients(t *testing.T) {
	backend := echoBackend(t)
	f, address, _ := launch(t, backend.Addr().String(), 1)
	hold := connect(t, address)
	if _, err := io.WriteString(hold, "x"); err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadFull(hold, make([]byte, 1))
	// Slot is full: a probe is rejected like any other client, no reply.
	c := connect(t, address)
	_, _ = io.WriteString(c, testProbe)
	if reply, _ := io.ReadAll(c); len(reply) != 0 {
		t.Fatalf("probe answered beyond MaxClients: %q", reply)
	}
	if f.RejectedConnections() != 1 {
		t.Fatalf("rejected=%d", f.RejectedConnections())
	}
	_ = hold.Close()
	deadline := time.Now().Add(time.Second)
	for f.ActiveConnections() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	// Probes release the single slot immediately, so they can run back to back.
	for i := 0; i < 3; i++ {
		p := connect(t, address)
		_, _ = io.WriteString(p, testProbe)
		got := make([]byte, len(testProbe))
		if _, err := io.ReadFull(p, got); err != nil || string(got) != testProbe {
			t.Fatalf("probe %d: %q %v", i, got, err)
		}
		_, _ = io.ReadAll(p)
	}
}

func TestProbeRepliesAreRateLimitedPerSource(t *testing.T) { eachLauncher(t, testRateLimited) }

func testRateLimited(t *testing.T, l launcher) {
	launch := l.run
	backend := newRecordingBackend(t, "")
	_, address, _ := launch(t, backend.ln.Addr().String(), 8)
	answered := 0
	for i := 0; i < probeBurst+5; i++ {
		c := connect(t, address)
		_, _ = io.WriteString(c, testProbe)
		got, _ := io.ReadAll(c)
		if len(got) > len(testProbe) {
			t.Fatalf("reply longer than frame: %d", len(got))
		}
		if string(got) == testProbe {
			answered++
		}
	}
	if answered < probeBurst || answered > probeBurst+1 {
		t.Fatalf("answered %d, want about burst %d", answered, probeBurst)
	}
	time.Sleep(100 * time.Millisecond)
	if backend.accepts.Load() != 0 {
		t.Fatal("limited probe leaked to target")
	}
}

func TestProbeLimiterBoundedAndRefills(t *testing.T) {
	var l probeLimiter
	now := time.Now()
	a := netip.MustParseAddr("192.0.2.1")
	for i := 0; i < probeBurst; i++ {
		if !l.allow(a, now) {
			t.Fatalf("burst %d denied", i)
		}
	}
	if l.allow(a, now) {
		t.Fatal("over burst allowed")
	}
	if !l.allow(a, now.Add(probeRefill)) {
		t.Fatal("no refill")
	}
	for i := 0; i < probeMaxSources+10; i++ {
		l.allow(netip.AddrFrom4([4]byte{10, byte(i >> 16), byte(i >> 8), byte(i)}), now)
	}
	if len(l.sources) > probeMaxSources {
		t.Fatalf("unbounded: %d", len(l.sources))
	}
}

func TestForwarderProbeConcurrentRaceAndCancel(t *testing.T) { eachLauncher(t, testConcurrent) }

func testConcurrent(t *testing.T, l launcher) {
	launch := l.run
	backend := echoBackend(t)
	_, address, stop := launch(t, backend.Addr().String(), 64)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, err := net.DialTimeout("tcp", address, time.Second)
			if err != nil {
				return
			}
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(2 * time.Second))
			if i%2 == 0 {
				_, _ = io.WriteString(c, testProbe)
				_, _ = io.ReadAll(c)
			} else {
				_, _ = io.WriteString(c, "data")
				_, _ = io.ReadFull(c, make([]byte, 4))
			}
		}(i)
	}
	wg.Wait()
	// A client stuck inside the peek window must not outlive cancellation.
	idle := connect(t, address)
	stop()
	if _, err := idle.Read(make([]byte, 1)); err == nil {
		t.Fatal("client survived shutdown")
	}
}
