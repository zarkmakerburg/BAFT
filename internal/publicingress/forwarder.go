// Package publicingress provides an isolated TCP forwarding runtime. It does
// not reserve ports in BCC, authorize jobs, manage ownership or change firewall
// policy. A bound listener is not proof of public reachability or tunnel health.
package publicingress

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type Config struct {
	Listen      string
	Target      string
	MaxClients  int
	DialTimeout time.Duration
	IdleTimeout time.Duration // Maximum silence across both directions; zero uses 5 minutes
}

// Forwarder owns one listener while running. The configuration is immutable;
// each admitted client forwards to exactly one literal loopback endpoint.
type Forwarder struct {
	cfg           Config
	listenIP      net.IP
	listenPort    int
	listenNetwork string
	targetNetwork string
	mu            sync.Mutex
	running       bool
	active        atomic.Int64
	rejected      atomic.Uint64
}

func literalEndpoint(raw string) (net.IP, int, error) {
	host, portText, err := net.SplitHostPort(raw)
	if err != nil {
		return nil, 0, errors.New("endpoint requires a literal IP and explicit port")
	}
	ip := net.ParseIP(host)
	port, portErr := strconv.Atoi(portText)
	if ip == nil || portErr != nil || port < 1 || port > 65535 {
		return nil, 0, errors.New("endpoint requires a literal IP and port between 1 and 65535")
	}
	return ip, port, nil
}

func New(cfg Config) (*Forwarder, error) {
	ip, port, err := literalEndpoint(cfg.Listen)
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	// Port 443 is an explicitly approved LAB ingress exception. Other
	// privileged ports remain fenced (including SSH management port 22).
	// The OS independently enforces permission to bind port 443.
	if port < 1024 && port != 443 {
		return nil, errors.New("privileged listen port is not allowed (except 443)")
	}
	// An ordinary service's public port is not a permanent magic number.
	// The kernel refuses conflicts with live listeners. Persisted claims and
	// stopped-service ownership must be verified by the admission layer.
	if !ip.IsUnspecified() && !ip.IsGlobalUnicast() && !ip.IsLoopback() {
		return nil, errors.New("listen IP must be unicast, loopback or wildcard")
	}
	target, targetPort, err := literalEndpoint(cfg.Target)
	if err != nil {
		return nil, fmt.Errorf("target: %w", err)
	}
	if !target.IsLoopback() {
		return nil, errors.New("target must be a literal loopback IP")
	}
	if targetPort == 22 {
		return nil, errors.New("target port 22 is reserved for SSH management")
	}
	if port == targetPort {
		return nil, errors.New("listen and target ports must differ to prevent forwarding loops")
	}
	if cfg.MaxClients == 0 {
		cfg.MaxClients = 128
	}
	if cfg.MaxClients < 1 || cfg.MaxClients > 1024 {
		return nil, errors.New("max clients must be between 1 and 1024")
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = 3 * time.Second
	}
	if cfg.DialTimeout < 10*time.Millisecond || cfg.DialTimeout > 10*time.Second {
		return nil, errors.New("dial timeout must be between 10ms and 10s")
	}
	if cfg.IdleTimeout == 0 {
		cfg.IdleTimeout = 5 * time.Minute
	}
	if cfg.IdleTimeout < 50*time.Millisecond || cfg.IdleTimeout > 30*time.Minute {
		return nil, errors.New("idle timeout must be between 50ms and 30m")
	}
	cfg.Listen = net.JoinHostPort(ip.String(), strconv.Itoa(port))
	cfg.Target = net.JoinHostPort(target.String(), strconv.Itoa(targetPort))
	network := func(ip net.IP) string {
		if ip.To4() != nil {
			return "tcp4"
		}
		return "tcp6"
	}
	return &Forwarder{cfg: cfg, listenIP: ip, listenPort: port, listenNetwork: network(ip), targetNetwork: network(target)}, nil
}

func (f *Forwarder) ActiveConnections() int64    { return f.active.Load() }
func (f *Forwarder) RejectedConnections() uint64 { return f.rejected.Load() }

// Run binds only the configured endpoint. A bind conflict leaves the existing
// listener untouched. Context cancellation is a clean shutdown, not an error.
func (f *Forwarder) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("ingress context is required")
	}
	if ctx.Err() != nil {
		return nil
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, f.listenNetwork, f.cfg.Listen)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("ingress listen: %w", err)
	}
	defer ln.Close()
	return f.Serve(ctx, ln)
}

// ValidateListener is read-only admission for a caller-owned bound socket.
// A failed preflight never changes or closes the listener. This lets the
// ingress runtime reject a foreign numeric-port owner BEFORE it persists a
// generation-fenced ownership claim or acknowledges the operation.
// A successful snapshot is NOT a durable reservation against later binds.
func (f *Forwarder) ValidateListener(ln net.Listener) error {
	if f == nil {
		return errors.New("ingress forwarder is required")
	}
	if ln == nil {
		return errors.New("ingress listener is required")
	}
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok || addr.Port != f.listenPort || !addr.IP.Equal(f.listenIP) {
		return errors.New("listener does not match configured ingress endpoint")
	}
	// Inspect both Linux TCP socket tables; unlike net.Listen success this
	// detects another live owner on a different IP or address family.
	return verifyNoOtherLiveTCPListeners(ln, f.listenPort)
}

// Serve takes ownership of an already-bound matching TCP listener after
// validation. A rejected or mismatched listener remains the caller's resource.
// This also allows tests to retain socket ownership without bind/close races.
func (f *Forwarder) Serve(parent context.Context, ln net.Listener) error {
	if parent == nil {
		return errors.New("ingress context is required")
	}
	if parent.Err() != nil {
		// Like Run on an already-canceled context, this is a clean
		// shutdown. No listener was accepted or owned, so it remains
		// the caller's responsibility to close it.
		return nil
	}
	if err := f.ValidateListener(ln); err != nil {
		return err
	}
	f.mu.Lock()
	if f.running {
		f.mu.Unlock()
		return errors.New("ingress forwarder is already running")
	}
	f.running = true
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.running = false; f.mu.Unlock() }()
	ctx, cancel := context.WithCancel(parent)
	var clients sync.WaitGroup
	stopListener := context.AfterFunc(ctx, func() { _ = ln.Close() })
	defer func() { cancel(); _ = ln.Close(); clients.Wait(); stopListener() }()
	slots := make(chan struct{}, f.cfg.MaxClients)
	for {
		client, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("ingress accept: %w", err)
		}
		if ctx.Err() != nil {
			_ = client.Close()
			return nil
		}
		select {
		case slots <- struct{}{}:
			f.active.Add(1)
			clients.Go(func() { defer func() { <-slots; f.active.Add(-1) }(); f.forward(ctx, client) })
		default:
			f.rejected.Add(1)
			_ = client.Close()
		}
	}
}

// activityReader records successful stream reads, regardless of direction.
// An idle peer cannot indefinitely exhaust the bounded connection budget.
// The window is refreshed by real bytes, not by a connection merely staying
// open; a write stalled by backpressure eventually expires too.
type activityReader struct {
	io.Reader
	touch func()
}

func (r activityReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if n > 0 {
		r.touch()
	}
	return n, err
}

func (f *Forwarder) forward(ctx context.Context, client net.Conn) {
	defer client.Close()
	stopClient := context.AfterFunc(ctx, func() { _ = client.Close() })
	defer stopClient()
	dialer := net.Dialer{Timeout: f.cfg.DialTimeout}
	upstream, err := dialer.DialContext(ctx, f.targetNetwork, f.cfg.Target)
	if err != nil {
		return
	}
	defer upstream.Close()
	stopUpstream := context.AfterFunc(ctx, func() { _ = upstream.Close() })
	defer stopUpstream()
	// One idle budget covers traffic in either direction. Timeout closes both
	// peers to release io.Copy goroutines and the admission slot. Successful
	// reads reset the timer; inactivity cannot keep a slot indefinitely.
	idle := time.AfterFunc(f.cfg.IdleTimeout, func() {
		_ = client.Close()
		_ = upstream.Close()
	})
	defer idle.Stop()
	touch := func() { idle.Reset(f.cfg.IdleTimeout) }
	// EOF half-closes only the destination write side. The opposite direction
	// stays alive until its own EOF; request EOF may trigger a delayed response.
	copyDirection := func(dst, src net.Conn) {
		if _, err := io.Copy(dst, activityReader{Reader: src, touch: touch}); err != nil {
			_ = client.Close()
			_ = upstream.Close()
			return
		}
		if tcp, ok := dst.(interface{ CloseWrite() error }); ok {
			if err := tcp.CloseWrite(); err == nil {
				return
			}
		}
		_ = client.Close()
		_ = upstream.Close()
	}
	done := make(chan struct{})
	go func() { defer close(done); copyDirection(client, upstream) }()
	copyDirection(upstream, client)
	<-done
}
