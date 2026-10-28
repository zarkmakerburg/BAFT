package failover

import (
	"context"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

// throttledEcho echoes only the first `limit` bytes of each connection and
// then swallows everything, like the path that stops after ~8 KiB. When
// open is false it echoes everything.
func throttledEcho(t *testing.T, limit int64, throttled *atomic.Bool) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				if throttled.Load() {
					_, _ = io.CopyN(c, c, limit)
					_, _ = io.Copy(io.Discard, c)
					return
				}
				_, _ = io.Copy(c, c)
			}(c)
		}
	}()
	return ln.Addr().String()
}

func echoAddr(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	addr := reserve(t)
	go func() { _ = ServeEcho(ctx, addr) }()
	for i := 0; i < 100; i++ {
		if c, err := net.Dial("tcp", addr); err == nil {
			c.Close()
			return addr
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("echo did not start")
	return ""
}

func TestHealthProbeSkipsPathThatStopsAfterFewKilobytes(t *testing.T) {
	var throttled atomic.Bool
	throttled.Store(true)
	primary := throttledEcho(t, 8088, &throttled)
	backup := echoAddr(t)
	var changes []string
	p := &Proxy{Listen: reserve(t), DialTimeout: 200 * time.Millisecond,
		Targets: []Target{{Name: "ex1", Address: primary, Probe: primary}, {Name: "ex2", Address: backup, Probe: backup}},
		Health: HealthConfig{Interval: time.Hour, Timeout: 300 * time.Millisecond, FailAfter: 2, RecoverAfter: 2,
			OnChange: func(n string, ok bool) { changes = append(changes, n+map[bool]string{true: "+", false: "-"}[ok]) }}}
	if err := p.validate(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.startHealth(ctx)
	time.Sleep(50 * time.Millisecond) // first ProbeOnce from the loop
	p.ProbeOnce(ctx)
	if h := p.Healthy(); h[0] || !h[1] {
		t.Fatalf("after 2 failed probes health = %v", h)
	}
	_, idx, err := p.dial(ctx)
	if err != nil || idx != 1 {
		t.Fatalf("dial chose %d (%v), want backup", idx, err)
	}

	throttled.Store(false)
	p.ProbeOnce(ctx)
	if p.Healthy()[0] {
		t.Fatal("recovered after one good probe; recover_after is 2")
	}
	p.ProbeOnce(ctx)
	if !p.Healthy()[0] {
		t.Fatal("primary not restored after recover_after good probes")
	}
	if _, idx, _ := p.dial(ctx); idx != 0 {
		t.Fatalf("dial chose %d after recovery, want primary", idx)
	}
	if len(changes) != 2 || changes[0] != "ex1-" || changes[1] != "ex1+" {
		t.Fatalf("changes = %v", changes)
	}
}

func TestAllUnhealthyStillDials(t *testing.T) {
	var throttled atomic.Bool
	throttled.Store(true)
	a := throttledEcho(t, 100, &throttled)
	b := throttledEcho(t, 100, &throttled)
	p := &Proxy{Listen: reserve(t), DialTimeout: 200 * time.Millisecond,
		Targets: []Target{{Name: "a", Address: a, Probe: a}, {Name: "b", Address: b, Probe: b}},
		Health:  HealthConfig{Interval: time.Hour, Timeout: 200 * time.Millisecond, FailAfter: 1}}
	if err := p.validate(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.startHealth(ctx)
	time.Sleep(30 * time.Millisecond)
	p.ProbeOnce(ctx)
	if h := p.Healthy(); h[0] || h[1] {
		t.Fatalf("health = %v", h)
	}
	c, idx, err := p.dial(ctx)
	if err != nil || idx != 0 {
		t.Fatalf("all-unhealthy dial: idx=%d err=%v", idx, err)
	}
	c.Close()
}

func TestHealthConfigBounds(t *testing.T) {
	for _, h := range []HealthConfig{{Bytes: -1}, {Bytes: 5 << 20}, {Interval: time.Millisecond}, {Timeout: 2 * time.Minute}} {
		if err := h.normalize(); err == nil {
			t.Fatalf("accepted %+v", h)
		}
	}
	h := HealthConfig{}
	if err := h.normalize(); err != nil || h.Bytes != 32<<10 {
		t.Fatalf("defaults: %+v %v", h, err)
	}
}
