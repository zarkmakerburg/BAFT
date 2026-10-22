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

// Serve takes ownership of an already-bound matching TCP listener after
// validation. A rejected or mismatched listener remains the caller's resource.
// This also allows tests to retain socket ownership without bind/close races.
func (f *Forwarder) Serve(parent context.Context, ln net.Listener) error {
	if ln == nil {
		return errors.New("ingress listener is required")
	}
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok || addr.Port != f.listenPort || !addr.IP.Equal(f.listenIP) {
		return errors.New("listener does not match configured ingress endpoint")
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
	// EOF half-closes only the destination write side. The opposite direction
	// stays alive until its own EOF; request EOF may trigger a delayed response.
	copyDirection := func(dst, src net.Conn) {
		if _, err := io.Copy(dst, src); err != nil {
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
