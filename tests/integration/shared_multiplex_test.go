package integration_test

import (
	"context"
	"errors"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"math/rand"
	"net"
	"syscall"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/config"
	"github.com/zarkmakerburg/baft/internal/node"
	"github.com/zarkmakerburg/baft/internal/recordshape"
	"github.com/zarkmakerburg/baft/internal/resources"
	"github.com/zarkmakerburg/baft/internal/securityinternal"
)

// Shared transport multiplexing: ONE EX listener serves several tenants
// (authenticated IR identities), each reusing the same route id "port-443"
// towards its own target. See docs/design/shared-multiplexing.md.

type tenant struct {
	name     string
	identity string
	keyPath  string
	pub      string
	target   net.Listener
	accepted atomic.Int64
	local    string // IR-side route listen address
	metrics  string
}

type sharedEnv struct {
	t        *testing.T
	dir      string
	tls      config.TLS
	exKey    string
	exPub    string
	exAddr   string
	exMetric string
	conns    sync.Map // target-side conns, closed on cleanup
}

func newSharedEnv(t *testing.T) *sharedEnv {
	t.Helper()
	certs := testPKI(t)
	dir := t.TempDir()
	write := func(name string, b []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	ca := write("ca.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certs.caDER}))
	cert := write("server.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certs.server.Certificate[0]}))
	der, err := x509.MarshalPKCS8PrivateKey(certs.server.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	tlsKey := write("server.key", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	exKey, err := securityinternal.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	exPath := filepath.Join(dir, "ex-noise.json")
	if err := securityinternal.SaveKeyPair(exPath, exKey); err != nil {
		t.Fatal(err)
	}
	exPub, _ := securityinternal.EncodePublicKey(exKey.Public)
	return &sharedEnv{t: t, dir: dir, tls: config.TLS{MinVersion: "1.3", CAFile: ca, CertFile: cert, KeyFile: tlsKey},
		exKey: exPath, exPub: exPub, exAddr: reserveAddress(t), exMetric: reserveAddress(t)}
}

func (e *sharedEnv) newTenant(name string) *tenant {
	t := e.t
	t.Helper()
	k, err := securityinternal.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(e.dir, name+"-noise.json")
	if err := securityinternal.SaveKeyPair(p, k); err != nil {
		t.Fatal(err)
	}
	pub, _ := securityinternal.EncodePublicKey(k.Public)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tn := &tenant{name: name, identity: "urn:baft:node:ir-" + name, keyPath: p, pub: pub, target: ln,
		local: reserveAddress(t), metrics: reserveAddress(t)}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			e.conns.Store(c, struct{}{})
			go func() {
				defer c.Close()
				// Count only connections that carry data: readiness probes
				// dial and close without sending anything.
				one := make([]byte, 1)
				if _, err := io.ReadFull(c, one); err != nil {
					return
				}
				tn.accepted.Add(1)
				if _, err := c.Write(one); err != nil {
					return
				}
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	t.Cleanup(func() {
		ln.Close()
		e.conns.Range(func(k, _ any) bool { k.(net.Conn).Close(); return true })
	})
	return tn
}

// exConfig builds the single shared EX listener config for the tenants.
func (e *sharedEnv) exConfig(tenants []*tenant, mutate ...func(*config.Config)) config.Config {
	c := config.Config{
		SchemaVersion: config.SchemaVersion,
		Node:          config.Node{ID: "ex-shared", Role: "listener"},
		Server:        &config.Server{Listen: e.exAddr, ServerName: "ex.test"},
		TLS:           e.tls,
		Transport:     config.Transport{Primary: "h2", Shards: 1, Profile: "secure-fast"},
		Limits:        config.Limits{MaxFlows: 256, DataMemoryMiB: 256, ReceiveInitialKiB: 64, ReceiveMaxMiB: 16, ReplayMaxMiB: 16},
		Recovery:      config.Recovery{RetentionSeconds: 30},
		Management:    config.Management{UnixSocket: filepath.Join(e.dir, "ex.sock"), MetricsListen: e.exMetric},
		Logging:       config.Logging{Level: "info"},
		Noise:         &config.Noise{KeyFile: e.exKey, AllowedPeerPublicKeys: map[string]string{}, RecordShaping: recordshape.Config{}},
	}
	for _, tn := range tenants {
		c.Server.AllowedPeerIdentities = append(c.Server.AllowedPeerIdentities, tn.identity)
		c.Noise.AllowedPeerPublicKeys[tn.identity] = tn.pub
		c.Routes = append(c.Routes, config.Route{ID: "port-443", Direction: "inbound", Target: tn.target.Addr().String(), AllowedPeers: []string{tn.identity}})
	}
	for _, m := range mutate {
		m(&c)
	}
	if err := config.Validate(c); err != nil {
		e.t.Fatalf("EX config: %v", err)
	}
	return c
}

func (e *sharedEnv) irConfig(tn *tenant, extra ...config.Route) config.Config {
	c := config.Config{
		SchemaVersion: config.SchemaVersion,
		Node:          config.Node{ID: strings.TrimPrefix(tn.identity, "urn:baft:node:"), Role: "dialer"},
		Peer:          &config.Peer{Address: e.exAddr, ServerName: "ex.test", AllowedIdentity: "urn:baft:node:ex-shared"},
		TLS:           e.tls,
		Transport:     config.Transport{Primary: "h2", Shards: 1, Profile: "secure-fast"},
		Limits:        config.Limits{MaxFlows: 256, DataMemoryMiB: 256, ReceiveInitialKiB: 64, ReceiveMaxMiB: 16, ReplayMaxMiB: 16},
		Recovery:      config.Recovery{RetentionSeconds: 30},
		Management:    config.Management{UnixSocket: filepath.Join(e.dir, tn.name+".sock"), MetricsListen: tn.metrics},
		Logging:       config.Logging{Level: "info"},
		Noise:         &config.Noise{KeyFile: tn.keyPath, PeerPublicKey: e.exPub, RecordShaping: recordshape.Config{}},
		Routes:        []config.Route{{ID: "port-443", Listen: tn.local, RemoteRoute: "port-443", Direction: "outbound", TrafficClass: "interactive"}},
	}
	c.Routes = append(c.Routes, extra...)
	if err := config.Validate(c); err != nil {
		e.t.Fatalf("IR config: %v", err)
	}
	return c
}

type running struct {
	rt     *node.Runtime
	cancel context.CancelFunc
	done   chan error
}

func (r *running) stop(t *testing.T) error {
	r.cancel()
	select {
	case err := <-r.done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("runtime did not stop")
		return nil
	}
}

func startNode(t *testing.T, cfg config.Config, waitAddr string) *running {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r := &running{rt: node.NewRuntime(), cancel: cancel, done: make(chan error, 1)}
	// Pre-set so tests can read the slot pool without racing Run.
	slots, err := resources.NewFlowSlots(cfg.Limits.MaxFlows)
	if err != nil {
		t.Fatal(err)
	}
	r.rt.FlowSlots = slots
	go func() { r.done <- r.rt.Run(ctx, cfg) }()
	waitTCP(t, waitAddr, time.Now().Add(10*time.Second))
	t.Cleanup(cancel)
	return r
}

// streamer pushes pseudo-random bytes through a tunnel and checks the echo.
type streamer struct {
	conn     net.Conn
	sent     chan [sha256.Size]byte
	recvSum  chan [sha256.Size]byte
	received atomic.Int64
	stopOnce sync.Once
	stop     chan struct{}
	werr     chan error
}

func startStream(t *testing.T, addr string, seed int64) *streamer {
	t.Helper()
	var c net.Conn
	var err error
	for i := 0; i < 50; i++ {
		c, err = net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	s := &streamer{conn: c, sent: make(chan [sha256.Size]byte, 1), recvSum: make(chan [sha256.Size]byte, 1), stop: make(chan struct{}), werr: make(chan error, 2)}
	go func() {
		rng := rand.New(rand.NewSource(seed))
		h := sha256.New()
		buf := make([]byte, 16<<10)
		for {
			select {
			case <-s.stop:
				c.(*net.TCPConn).CloseWrite()
				var sum [sha256.Size]byte
				copy(sum[:], h.Sum(nil))
				s.sent <- sum
				return
			default:
			}
			rng.Read(buf)
			if _, err := c.Write(buf); err != nil {
				s.werr <- err
				var sum [sha256.Size]byte
				s.sent <- sum
				return
			}
			h.Write(buf)
			time.Sleep(time.Millisecond)
		}
	}()
	go func() {
		h := sha256.New()
		buf := make([]byte, 64<<10)
		for {
			n, err := c.Read(buf)
			h.Write(buf[:n])
			s.received.Add(int64(n))
			if err != nil {
				if err != io.EOF {
					s.werr <- err
				}
				break
			}
		}
		var sum [sha256.Size]byte
		copy(sum[:], h.Sum(nil))
		s.recvSum <- sum
	}()
	return s
}

func (s *streamer) waitProgress(t *testing.T, delta int64, d time.Duration) {
	t.Helper()
	start := s.received.Load()
	dl := time.Now().Add(d)
	for s.received.Load()-start < delta {
		select {
		case err := <-s.werr:
			t.Fatalf("stream broke while expecting progress: %v", err)
		default:
		}
		if time.Now().After(dl) {
			t.Fatalf("stream stalled: only %d of %d bytes in %v", s.received.Load()-start, delta, d)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// finish half-closes, waits for the echo to drain and verifies checksums.
func (s *streamer) finish(t *testing.T, label string) {
	t.Helper()
	s.stopOnce.Do(func() { close(s.stop) })
	var sent, got [sha256.Size]byte
	select {
	case sent = <-s.sent:
	case <-time.After(15 * time.Second):
		t.Fatalf("%s: writer did not finish", label)
	}
	select {
	case got = <-s.recvSum:
	case <-time.After(20 * time.Second):
		t.Fatalf("%s: echo did not drain", label)
	}
	select {
	case err := <-s.werr:
		t.Fatalf("%s: stream error: %v", label, err)
	default:
	}
	if sent != got {
		t.Fatalf("%s: checksum mismatch sent=%x recv=%x (received %d bytes)", label, sent[:6], got[:6], s.received.Load())
	}
	if s.received.Load() == 0 {
		t.Fatalf("%s: no bytes carried", label)
	}
	s.conn.Close()
}

func roundtripOnce(addr, label string) error {
	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	msg := []byte(strings.Repeat(label+"|", 64))
	if _, err := c.Write(msg); err != nil {
		return err
	}
	got := make([]byte, len(msg))
	if _, err := io.ReadFull(c, got); err != nil {
		return err
	}
	if string(got) != string(msg) {
		return fmt.Errorf("%s: echo mismatch", label)
	}
	return nil
}

// (a) Two tenants share one EX listener and carry independent traffic at once
// to their own targets, with the same route id.
func TestSharedListenerTwoTenantsIndependentConcurrentTraffic(t *testing.T) {
	e := newSharedEnv(t)
	a, b := e.newTenant("a"), e.newTenant("b")
	ex := startNode(t, e.exConfig([]*tenant{a, b}), e.exAddr)
	irA := startNode(t, e.irConfig(a), a.local)
	irB := startNode(t, e.irConfig(b), b.local)

	baseA, baseB := a.accepted.Load(), b.accepted.Load() // readiness probes dial the tunnel too
	sa, sb := startStream(t, a.local, 1), startStream(t, b.local, 2)
	sa.waitProgress(t, 1<<20, 20*time.Second)
	sb.waitProgress(t, 1<<20, 20*time.Second)
	sa.finish(t, "tenant a")
	sb.finish(t, "tenant b")

	if a.accepted.Load()-baseA != 1 || b.accepted.Load()-baseB != 1 {
		t.Fatalf("each tenant must reach only its own target once: a=%d b=%d", a.accepted.Load()-baseA, b.accepted.Load()-baseB)
	}
	for _, r := range []*running{irA, irB, ex} {
		if err := r.stop(t); err != nil {
			t.Fatalf("runtime stop: %v", err)
		}
	}
	if u := ex.rt.FlowSlots.Used(); u != 0 {
		t.Fatalf("flow slots leaked after all flows closed: %d", u)
	}
}

// (b) A third tenant is added live while two stream. Neither stream is
// interrupted and both keep their checksums.
func TestAddThirdTenantLiveDoesNotInterruptStreams(t *testing.T) {
	e := newSharedEnv(t)
	a, b, c := e.newTenant("a"), e.newTenant("b"), e.newTenant("c")
	ex := startNode(t, e.exConfig([]*tenant{a, b}), e.exAddr)
	startNode(t, e.irConfig(a), a.local)
	startNode(t, e.irConfig(b), b.local)
	baseA, baseB := a.accepted.Load(), b.accepted.Load()
	sa, sb := startStream(t, a.local, 11), startStream(t, b.local, 12)
	sa.waitProgress(t, 512<<10, 20*time.Second)
	sb.waitProgress(t, 512<<10, 20*time.Second)

	// The new tenant cannot connect before it is admitted.
	irCcfg := e.irConfig(c)
	if _, err := ex.rt.ReloadListener(e.exConfig([]*tenant{a, b, c})); err != nil {
		t.Fatalf("reload: %v", err)
	}
	startNode(t, irCcfg, c.local)
	var last error
	for i := 0; i < 50; i++ {
		if last = roundtripOnce(c.local, "c"); last == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if last != nil {
		t.Fatalf("new tenant never became usable: %v", last)
	}
	sa.waitProgress(t, 512<<10, 20*time.Second)
	sb.waitProgress(t, 512<<10, 20*time.Second)
	sa.finish(t, "tenant a")
	sb.finish(t, "tenant b")
	if a.accepted.Load()-baseA != 1 || b.accepted.Load()-baseB != 1 || c.accepted.Load() < 1 {
		t.Fatalf("targets: a=%d b=%d c=%d", a.accepted.Load()-baseA, b.accepted.Load()-baseB, c.accepted.Load())
	}
}

// (c) Removing one tenant (reload) or crashing its IR leaves the others up.
func TestRemoveOrCrashOneTenantLeavesOthersUp(t *testing.T) {
	e := newSharedEnv(t)
	a, b, c := e.newTenant("a"), e.newTenant("b"), e.newTenant("c")
	ex := startNode(t, e.exConfig([]*tenant{a, b, c}), e.exAddr)
	startNode(t, e.irConfig(a), a.local)
	irB := startNode(t, e.irConfig(b), b.local)
	irC := startNode(t, e.irConfig(c), c.local)
	sa, sb, sc := startStream(t, a.local, 21), startStream(t, b.local, 22), startStream(t, c.local, 23)
	for _, s := range []*streamer{sa, sb, sc} {
		s.waitProgress(t, 256<<10, 20*time.Second)
	}

	// Crash tenant b's IR abruptly.
	irB.cancel()
	<-irB.done
	sa.waitProgress(t, 512<<10, 20*time.Second)
	sc.waitProgress(t, 512<<10, 20*time.Second)

	// Remove tenant c from the shared listener without restarting it.
	res, err := ex.rt.ReloadListener(e.exConfig([]*tenant{a, b}))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Removed) != 1 || res.Removed[0] != c.identity {
		t.Fatalf("removed=%v", res.Removed)
	}
	select {
	case <-irC.done: // its carrier was cut: it exits like any lost dialer
	case <-time.After(10 * time.Second):
		t.Fatal("removed tenant's session was not cut")
	}
	sa.waitProgress(t, 512<<10, 20*time.Second)
	sa.finish(t, "tenant a")
	_ = sb.conn.Close()
	_ = sc.conn.Close()
}

// (d) A tenant naming another tenant's route is refused on the EX and never
// reaches the other tenant's target.
func TestCrossTenantRouteAccessRefused(t *testing.T) {
	e := newSharedEnv(t)
	a, b := e.newTenant("a"), e.newTenant("b")
	ex := e.exConfig([]*tenant{a, b}, func(c *config.Config) {
		c.Routes = append(c.Routes, config.Route{ID: "only-a", Direction: "inbound", Target: a.target.Addr().String(), AllowedPeers: []string{a.identity}})
	})
	startNode(t, ex, e.exAddr)
	steal := reserveAddress(t)
	unknown := reserveAddress(t)
	irBcfg := e.irConfig(b,
		config.Route{ID: "steal", Listen: steal, RemoteRoute: "only-a", Direction: "outbound"},
		config.Route{ID: "ghost", Listen: unknown, RemoteRoute: "no-such-route", Direction: "outbound"})
	irB := startNode(t, irBcfg, b.local)
	// The route listeners bind after the node reports its local address; a dial
	// that races them is reset by the kernel (flake seen on CI).
	waitTCP(t, steal, time.Now().Add(10*time.Second))
	waitTCP(t, unknown, time.Now().Add(10*time.Second))
	var mu sync.Mutex
	var errs []string
	irB.rt.SetFlowOpenErrorHookForTest(func(route string, err error) {
		mu.Lock()
		errs = append(errs, route+":"+err.Error())
		mu.Unlock()
	})
	startNode(t, e.irConfig(a), a.local)
	if err := roundtripOnce(a.local, "a"); err != nil {
		t.Fatal(err)
	}
	base := a.accepted.Load()

	for _, addr := range []string{steal, unknown} {
		c, err := net.DialTimeout("tcp", addr, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		_ = c.SetDeadline(time.Now().Add(5 * time.Second))
		_, _ = c.Write([]byte("probe"))
		if n, err := c.Read(make([]byte, 8)); err == nil || n != 0 {
			t.Fatalf("refused flow carried data: n=%d err=%v", n, err)
		}
		c.Close()
	}
	if a.accepted.Load() != base {
		t.Fatal("cross-tenant flow reached tenant a's target")
	}
	var joined string
	for i := 0; i < 50; i++ { // the hook runs just after the client side is closed
		mu.Lock()
		joined = strings.Join(errs, ";")
		mu.Unlock()
		if strings.Contains(joined, "ROUTE_DENIED") && strings.Contains(joined, "ROUTE_NOT_FOUND") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !strings.Contains(joined, "steal:") || !strings.Contains(joined, "ROUTE_DENIED") || !strings.Contains(joined, "ROUTE_NOT_FOUND") {
		t.Fatalf("expected EX refusals ROUTE_DENIED and ROUTE_NOT_FOUND, got %q", joined)
	}
	// Tenant b's own route still works.
	if err := roundtripOnce(b.local, "b"); err != nil {
		t.Fatal(err)
	}
}

// (e) Per-tenant flow cap: one tenant cannot take more than its share, and
// other tenants are unaffected. The cap frees up when flows close.
func TestPerTenantFlowCapEnforced(t *testing.T) {
	e := newSharedEnv(t)
	a, b := e.newTenant("a"), e.newTenant("b")
	ex := startNode(t, e.exConfig([]*tenant{a, b}, func(c *config.Config) { c.Limits.MaxFlowsPerPeer = 2 }), e.exAddr)
	startNode(t, e.irConfig(a), a.local)
	startNode(t, e.irConfig(b), b.local)

	hold := func(addr string) net.Conn {
		c, err := net.DialTimeout("tcp", addr, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		_ = c.SetDeadline(time.Now().Add(8 * time.Second))
		if _, err := c.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
		if _, err := io.ReadFull(c, make([]byte, 1)); err != nil {
			t.Fatalf("flow within cap failed: %v", err)
		}
		return c
	}
	a1, a2 := hold(a.local), hold(a.local)
	// Third flow of tenant a is refused.
	c3, err := net.DialTimeout("tcp", a.local, time.Second)
	if err != nil {
		// The listener rejects a capped OPEN with TypeOpenErr. That can close
		// the local socket before DialTimeout returns, surfacing the same
		// intentional refusal as ECONNRESET instead of a readable connection.
		if !errors.Is(err, syscall.ECONNRESET) {
			t.Fatalf("third flow returned unexpected dial error: %v", err)
		}
		t.Logf("third flow correctly refused during dial: %v", err)
	} else {
		_ = c3.SetDeadline(time.Now().Add(5 * time.Second))
		_, _ = c3.Write([]byte("x"))
		if n, err := c3.Read(make([]byte, 1)); err == nil || n != 0 {
			t.Fatalf("flow beyond the per-tenant cap was admitted: n=%d err=%v", n, err)
		}
		_ = c3.Close()
	}
	// Tenant b is not affected by a's exhaustion.
	b1, b2 := hold(b.local), hold(b.local)
	if u := ex.rt.FlowSlots.Used(); u != 4 {
		t.Fatalf("node slots used=%d want 4", u)
	}
	// Closing one of a's flows frees a slot for a.
	a1.Close()
	var last error
	for i := 0; i < 50; i++ {
		if last = roundtripOnce(a.local, "again"); last == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if last != nil {
		t.Fatalf("slot not released after close: %v", last)
	}
	for _, c := range []net.Conn{a2, b1, b2} {
		c.Close()
	}
}

// (f) Restarting the shared EX: tenants re-establish and no state leaks.
func TestSharedListenerRestartReestablishesTenants(t *testing.T) {
	e := newSharedEnv(t)
	a, b := e.newTenant("a"), e.newTenant("b")
	cfg := e.exConfig([]*tenant{a, b})
	ex1 := startNode(t, cfg, e.exAddr)
	ir1A := startNode(t, e.irConfig(a), a.local)
	ir1B := startNode(t, e.irConfig(b), b.local)
	if err := roundtripOnce(a.local, "a"); err != nil {
		t.Fatal(err)
	}
	if err := roundtripOnce(b.local, "b"); err != nil {
		t.Fatal(err)
	}
	_ = ex1.stop(t)
	_ = ir1A.stop(t)
	_ = ir1B.stop(t)
	if u := ex1.rt.FlowSlots.Used(); u != 0 {
		t.Fatalf("flow slots leaked across shutdown: %d", u)
	}

	ex2 := startNode(t, cfg, e.exAddr)
	if u := ex2.rt.FlowSlots.Used(); u != 0 {
		t.Fatalf("restarted node carries state: %d slots", u)
	}
	startNode(t, e.irConfig(a), a.local)
	startNode(t, e.irConfig(b), b.local)
	var lastA, lastB error
	for i := 0; i < 50; i++ {
		lastA, lastB = roundtripOnce(a.local, "a2"), roundtripOnce(b.local, "b2")
		if lastA == nil && lastB == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if lastA != nil || lastB != nil {
		t.Fatalf("tenants did not re-establish: a=%v b=%v", lastA, lastB)
	}
}

// A reload that changes anything but the tenant set is refused and changes
// nothing; a reload on a node with no listener fails.
func TestReloadListenerRefusesNonTenantChanges(t *testing.T) {
	e := newSharedEnv(t)
	a, b := e.newTenant("a"), e.newTenant("b")
	ex := startNode(t, e.exConfig([]*tenant{a}), e.exAddr)
	startNode(t, e.irConfig(a), a.local)
	sa := startStream(t, a.local, 31)
	sa.waitProgress(t, 256<<10, 20*time.Second)

	for name, mutate := range map[string]func(*config.Config){
		"listen address": func(c *config.Config) { c.Server.Listen = reserveAddress(t) },
		"limits":         func(c *config.Config) { c.Limits.MaxFlows = 17 },
		"per-peer cap":   func(c *config.Config) { c.Limits.MaxFlowsPerPeer = 3 },
	} {
		if _, err := ex.rt.ReloadListener(e.exConfig([]*tenant{a, b}, mutate)); err == nil {
			t.Fatalf("%s: reload must be refused", name)
		}
	}
	bad := e.exConfig([]*tenant{a, b})
	bad.Routes = nil
	if _, err := ex.rt.ReloadListener(bad); err == nil {
		t.Fatal("config without routes must be refused")
	}
	// b was never admitted: its IR cannot establish a carrier and exits.
	ctxB, cancelB := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancelB()
	if err := node.NewRuntime().Run(ctxB, e.irConfig(b)); err == nil {
		t.Fatal("unadmitted tenant established a carrier after refused reloads")
	}
	if b.accepted.Load() != 0 {
		t.Fatal("unadmitted tenant reached its target")
	}
	sa.waitProgress(t, 256<<10, 20*time.Second)
	sa.finish(t, "tenant a")
	if _, err := node.NewRuntime().ReloadListener(e.exConfig([]*tenant{a})); err == nil {
		t.Fatal("reload without a running listener must fail")
	}
}
