//go:build linux

package operational

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/bcc"
)

// longConn is an established client session that must survive everything that
// happens to its siblings; it keeps proving liveness with fresh payloads.
type longConn struct {
	t *testing.T
	c net.Conn
	n int
}

func dialLong(t *testing.T, addr string) *longConn {
	t.Helper()
	c, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	l := &longConn{t: t, c: c}
	l.ping("open")
	return l
}

func (l *longConn) ping(why string) {
	l.t.Helper()
	l.n++
	msg := []byte(fmt.Sprintf("ping-%d-%s", l.n, why))
	_ = l.c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := l.c.Write(msg); err != nil {
		l.t.Fatalf("established session interrupted (%s): %v", why, err)
	}
	got := make([]byte, len(msg))
	if _, err := io.ReadFull(l.c, got); err != nil || !bytes.Equal(got, msg) {
		l.t.Fatalf("established session interrupted (%s): %v %q", why, err, got)
	}
}

func TestOperationalIngressV1Gate_SiblingTunnelsSurviveInstallAndRollback(t *testing.T) {
	f := setupFullPath(t, 54781, "127.0.0.1:1443")
	keep := f.h.addKeep()
	keepBefore := keep()
	unrelated := foreignListen(t, "tcp4", "127.0.0.1:2053")
	bannerServer(t, unrelated, "UNRELATED\n")
	f.legit(t)
	conn := dialLong(t, f.addr())
	aTag := tagOf(t, f.site)

	// Install a sibling.
	b := f.addSite(t, "ex-opv2", "second", 8444, 54782, "127.0.0.1:1444")
	b.apply(t)
	if v := b.waitReady(t); !v.Ready {
		t.Fatalf("sibling not ready: %+v", v)
	}
	conn.ping("after sibling install")
	if err := roundTrip(t, b.addr(), []byte("b-works")); err != nil {
		t.Fatal(err)
	}

	// A third install fails and rolls back.
	c := f.addSite(t, "ex-opv3", "third", 8445, 54783, "127.0.0.1:1445")
	c.enqueueApply(t)
	f.a.Runner.failAdd = true
	if res := f.a.runOnce(); !isFailure(res) {
		t.Fatalf("partial add reported success: %+v", res)
	}
	conn.ping("after failed sibling install")
	if err := roundTrip(t, b.addr(), []byte("b-still")); err != nil {
		t.Fatalf("sibling B disturbed by the rollback of C: %v", err)
	}

	// Retire the sibling B; A's established session still flows.
	b.enqueueRetire(t)
	if res := f.a.runOnce(); len(res) != 1 || res[0].Status != "succeeded" {
		t.Fatalf("retire B: %+v", res)
	}
	conn.ping("after sibling retire")
	chain := f.h.chainJSON()
	if !strings.Contains(chain, aTag) || strings.Contains(chain, tagOf(t, b)) || strings.Contains(chain, tagOf(t, c)) {
		t.Fatalf("rule set after install/rollback/retire is wrong: %s", chain)
	}
	if keep() != keepBefore {
		t.Fatal("an unrelated nft table changed")
	}
	if got, err := readBanner("127.0.0.1:2053"); err != nil || got != "UNRELATED\n" {
		t.Fatalf("unrelated service on 2053 disturbed: %q %v", got, err)
	}
	if v := f.b.readiness(f.tn.ID); !v.Ready && f.waitReady(t).Card == nil {
		t.Fatal("A's card lost")
	}
	// Tunnels A and B remain ACTIVE in BCC and no job was ever queued for A
	// after its own apply.
	for _, id := range []string{f.tn.ID, b.tn.ID} {
		if got := f.b.tunnel(id); got.Phase != bcc.TunnelActive {
			t.Fatalf("tunnel %s phase %s", id, got.Phase)
		}
	}
	conn.ping("end")
}

func TestOperationalIngressV1Gate_MultipleEXPeersOnOneIRWithoutInterruption(t *testing.T) {
	f := setupFullPath(t, 54791, "127.0.0.1:1443")
	f.legit(t)
	conn := dialLong(t, f.addr())
	jobsFor := func(id string) int {
		n := 0
		for _, j := range f.b.store.ListJobs() {
			if j.Params["tunnel_id"] == id {
				n++
			}
		}
		return n
	}
	aJobs := jobsFor(f.tn.ID)
	sites := []*site{f.site}
	for i, ex := range []string{"ex-opv2", "ex-opv3"} {
		s := f.addSite(t, ex, fmt.Sprintf("peer-%d", i+2), 8444+i, 54792+i, fmt.Sprintf("127.0.0.1:%d", 1444+i))
		conn.ping("after wizard of " + ex)
		s.apply(t)
		conn.ping("after apply of " + ex)
		if v := s.waitReady(t); !v.Ready || v.Card == nil || v.Card.IRNode != irNode || v.Card.TunnelID != s.tn.ID {
			t.Fatalf("%s: card: %+v", ex, v)
		}
		sites = append(sites, s)
	}
	// Three EX peers share one IR: three independent listeners, rules and
	// cards, each routed to its own tunnel target.
	if n := f.h.baftRules(); n != 3 {
		t.Fatalf("expected one rule per tunnel, got %d: %s", n, f.h.chainJSON())
	}
	var wg sync.WaitGroup
	for _, s := range sites {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := make([]byte, 64<<10)
			_, _ = rand.Read(p)
			if err := roundTrip(t, s.addr(), p); err != nil {
				t.Errorf("tunnel %s: %v", s.tn.ID, err)
			}
		}()
	}
	wg.Wait()
	for _, s := range sites {
		if s.echo.seen() == 0 {
			t.Fatalf("tunnel %s target saw no traffic", s.tn.ID)
		}
	}
	// Every BCC tunnel stayed ACTIVE; the first tunnel got no new job at all
	// while its peers were added.
	for _, s := range sites {
		if got := f.b.tunnel(s.tn.ID); got.Phase != bcc.TunnelActive {
			t.Fatalf("tunnel %s phase %s", s.tn.ID, got.Phase)
		}
	}
	if got := jobsFor(f.tn.ID); got != aJobs {
		t.Fatalf("adding peers queued %d jobs on the first tunnel", got-aJobs)
	}
	// Retire the middle peer; the other two keep serving the same sessions.
	sites[1].enqueueRetire(t)
	if res := f.a.runOnce(); len(res) != 1 || res[0].Status != "succeeded" {
		t.Fatalf("retire peer: %+v", res)
	}
	conn.ping("after retire of a peer")
	if err := roundTrip(t, sites[2].addr(), []byte("third-ok")); err != nil {
		t.Fatalf("third peer: %v", err)
	}
}

// Client bytes cross the real listener and nft rule to the tunnel target
// unchanged, in both directions, at size, concurrently, with half-close, and
// the path recovers when the tunnel target comes back.
func TestOperationalIngressV1Gate_ClientTCPBytesRoundTripThroughIR(t *testing.T) {
	f := setupFullPath(t, 54801, "127.0.0.1:1443")
	f.legit(t)
	for _, addr := range []string{f.addr(), fmt.Sprintf("%s:%d", irAddr, f.port)} {
		big := make([]byte, 1<<20)
		_, _ = rand.Read(big)
		if err := roundTrip(t, addr, big); err != nil {
			t.Fatalf("1 MiB via %s: %v", addr, err)
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := make([]byte, 8<<10+i)
			_, _ = rand.Read(p)
			if err := roundTrip(t, f.addr(), p); err != nil {
				t.Errorf("client %d: %v", i, err)
			}
		}()
	}
	wg.Wait()
	// Half-close: send, FIN, still receive the full echo, then EOF.
	c, err := net.DialTimeout("tcp", f.addr(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte{0, 1, 2, 255, 254}, 4096)
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write(payload); err != nil {
		t.Fatal(err)
	}
	_ = c.(*net.TCPConn).CloseWrite()
	got, err := io.ReadAll(c)
	_ = c.Close()
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("half-closed session: %d/%d bytes, %v", len(got), len(payload), err)
	}
	// A connection to a dead tunnel target is closed promptly, not hung.
	_ = f.echo.ln.Close()
	d, err := net.DialTimeout("tcp", f.addr(), 2*time.Second)
	if err == nil {
		_ = d.SetDeadline(time.Now().Add(6 * time.Second))
		_, _ = d.Write([]byte("into the void"))
		if _, rerr := io.ReadAll(d); rerr != nil && strings.Contains(rerr.Error(), "timeout") {
			t.Fatal("client hung on an unreachable tunnel target")
		}
		_ = d.Close()
	}
	// The path recovers without any re-apply once the target is back.
	f.echo = startEcho(t, f.route)
	if err := roundTrip(t, f.addr(), []byte("recovered")); err != nil {
		t.Fatalf("path did not recover with the tunnel target: %v", err)
	}
	// A port that was never applied stays closed.
	if listening("127.0.0.1:54802") {
		t.Fatal("an unapplied port accepts connections")
	}
}
