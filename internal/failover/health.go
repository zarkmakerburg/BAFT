package failover

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

// HealthConfig controls active end-to-end probing of targets that have a
// Probe address. Zero values take the defaults below.
type HealthConfig struct {
	Interval     time.Duration // default 5s
	Timeout      time.Duration // default 5s, for one whole probe
	Bytes        int           // default 32 KiB: above the ~8 KiB cut-off seen on throttled paths
	FailAfter    int           // default 2 consecutive failed probes
	RecoverAfter int           // default 3 consecutive good probes
	OnChange     func(name string, healthy bool)
}

func (h *HealthConfig) normalize() error {
	if h.Interval == 0 {
		h.Interval = 5 * time.Second
	}
	if h.Timeout == 0 {
		h.Timeout = 5 * time.Second
	}
	if h.Bytes == 0 {
		h.Bytes = 32 << 10
	}
	if h.FailAfter == 0 {
		h.FailAfter = 2
	}
	if h.RecoverAfter == 0 {
		h.RecoverAfter = 3
	}
	switch {
	case h.Interval < 100*time.Millisecond || h.Interval > time.Hour:
		return errors.New("invalid failover probe interval")
	case h.Timeout < 100*time.Millisecond || h.Timeout > time.Minute:
		return errors.New("invalid failover probe timeout")
	case h.Bytes < 1 || h.Bytes > 4<<20:
		return errors.New("failover probe bytes must be 1..4MiB")
	case h.FailAfter < 1 || h.RecoverAfter < 1:
		return errors.New("failover fail/recover counts must be at least 1")
	}
	return nil
}

type healthState struct {
	mu      sync.Mutex
	healthy []bool
	streak  []int
}

// healthy reports whether target i may be preferred. Targets without a probe
// are always healthy (the dial result is their only signal).
func (p *Proxy) healthy(i int) bool {
	if p.health == nil {
		return true
	}
	p.health.mu.Lock()
	defer p.health.mu.Unlock()
	return p.health.healthy[i]
}

// Healthy returns the current health of every target, in order.
func (p *Proxy) Healthy() []bool {
	out := make([]bool, len(p.Targets))
	for i := range out {
		out[i] = p.healthy(i)
	}
	return out
}

func (p *Proxy) startHealth(ctx context.Context) {
	probed := false
	for _, t := range p.Targets {
		probed = probed || t.Probe != ""
	}
	if !probed {
		return
	}
	n := len(p.Targets)
	p.health = &healthState{healthy: make([]bool, n), streak: make([]int, n)}
	for i := range p.health.healthy {
		p.health.healthy[i] = true
	}
	go func() {
		t := time.NewTicker(p.Health.Interval)
		defer t.Stop()
		for {
			p.ProbeOnce(ctx)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

// ProbeOnce probes every target with a Probe address once, concurrently.
func (p *Proxy) ProbeOnce(ctx context.Context) {
	if p.health == nil {
		return
	}
	var wg sync.WaitGroup
	for i, t := range p.Targets {
		if t.Probe == "" {
			continue
		}
		wg.Add(1)
		go func(i int, addr string) {
			defer wg.Done()
			p.record(i, probeEcho(ctx, addr, p.Health.Bytes, p.Health.Timeout) == nil)
		}(i, t.Probe)
	}
	wg.Wait()
}

func (p *Proxy) record(i int, ok bool) {
	h := p.health
	h.mu.Lock()
	if ok == h.healthy[i] {
		h.streak[i] = 0
		h.mu.Unlock()
		return
	}
	h.streak[i]++
	need := p.Health.FailAfter
	if !h.healthy[i] {
		need = p.Health.RecoverAfter
	}
	changed := h.streak[i] >= need
	if changed {
		h.healthy[i] = ok
		h.streak[i] = 0
	}
	h.mu.Unlock()
	if changed && p.Health.OnChange != nil {
		p.Health.OnChange(p.Targets[i].Name, ok)
	}
}

// probeEcho sends n random bytes to an echo endpoint and requires the same
// bytes back before the deadline.
func probeEcho(ctx context.Context, addr string, n int, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var d net.Dialer
	c, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer c.Close()
	deadline, _ := ctx.Deadline()
	_ = c.SetDeadline(deadline)
	want := make([]byte, n)
	if _, err := rand.Read(want); err != nil {
		return err
	}
	errc := make(chan error, 1)
	go func() { _, err := c.Write(want); errc <- err }()
	got := make([]byte, n)
	if _, err := io.ReadFull(c, got); err != nil {
		return err
	}
	if err := <-errc; err != nil {
		return err
	}
	if !bytes.Equal(got, want) {
		return errors.New("probe echo mismatch")
	}
	return nil
}

// ServeEcho runs a plain TCP echo endpoint for probes until ctx ends.
func ServeEcho(ctx context.Context, listen string) error {
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	go func() { <-ctx.Done(); _ = ln.Close() }()
	for {
		c, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go func(c net.Conn) {
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(time.Minute))
			_, _ = io.Copy(c, io.LimitReader(c, 8<<20))
		}(c)
	}
}
