package ingressownership

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"github.com/zarkmakerburg/baft/internal/publicingress"
)

// Runtime owns only sockets it bound itself. This lab-only executor never
// changes host firewall policy and must not be wired to signed deploy jobs.
var ErrStopped = errors.New("ingress session has stopped; explicit retire required")

type Runtime struct {
	mu       sync.Mutex
	registry *Registry
	sessions map[string]*session
}
type session struct {
	proof    ingressmanifest.Proof
	cancel   context.CancelFunc
	done     chan error
	retiring bool
}

func NewRuntime() *Runtime { return &Runtime{registry: New(), sessions: make(map[string]*session)} }

// canonicalEndpoint matches the manifest's canonical IP and decimal port.
func canonicalEndpoint(endpoint string) (string, error) {
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		return "", ErrInvalid
	}
	ip := net.ParseIP(host)
	n, err := strconv.Atoi(port)
	if ip == nil || err != nil || n < 1 || n > 65535 {
		return "", ErrInvalid
	}
	return net.JoinHostPort(ip.String(), strconv.Itoa(n)), nil
}

// Apply binds before recording ownership; a bind conflict never touches the
// existing service. All session transitions are serialized.
func (r *Runtime) Apply(parent context.Context, spec ingressmanifest.Spec, proof ingressmanifest.Proof) error {
	if r == nil || parent == nil {
		return ErrInvalid
	}
	if err := parent.Err(); err != nil {
		return err
	}
	expected, err := ingressmanifest.Preview(spec)
	if err != nil {
		return err
	}
	if expected != proof {
		return ErrInvalid
	}
	canonical, err := canonicalEndpoint(spec.Config.Listen)
	if err != nil {
		return err
	}
	spec.Config.Listen = canonical
	f, err := publicingress.New(publicingress.Config{Listen: spec.Config.Listen, Target: spec.Config.Target, MaxClients: spec.Config.MaxClients, DialTimeout: time.Duration(spec.Config.DialTimeoutMS) * time.Millisecond})
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// A zero-value LAB runtime must retain exact-owner generation fences.
	if r.registry == nil {
		r.registry = New()
	}
	if r.sessions == nil {
		r.sessions = make(map[string]*session)
	}
	if s, ok := r.sessions[spec.Config.Listen]; ok {
		if s.proof == proof && !s.retiring {
			select {
			case <-s.done:
				return ErrStopped
			default:
				// Cancellation may race with the initial check. Never ACK
				// a cancelled replay as an active, healthy listener.
				if err := parent.Err(); err != nil {
					return err
				}
				return nil
			}
		}
		return ErrConflict
	}
	// Bind before claiming: an unrelated process holding the port must not
	// consume the owner's generation or leave a false reservation.
	ln, err := net.Listen("tcp", spec.Config.Listen)
	if err != nil {
		return fmt.Errorf("ingress bind: %w", err)
	}
	if err := r.registry.Claim(spec.Config.Listen, proof); err != nil {
		_ = ln.Close()
		return err
	}
	// After claiming a generation, never roll it back as if unused:
	// doing so would consume the high-water mark without a session to retire.
	// A concurrently cancelled parent is represented by a stopped session,
	// which can only be retired with its exact proof.
	ctx, cancel := context.WithCancel(parent)
	s := &session{proof: proof, cancel: cancel, done: make(chan error, 1)}
	r.sessions[spec.Config.Listen] = s
	go func() {
		err := f.Serve(ctx, ln)
		r.mu.Lock()
		// Retain a completed session until explicit exact-owner retirement.
		// Otherwise an unexpected exit could allow a stale same-proof retry.
		s.done <- err
		close(s.done)
		r.mu.Unlock()
	}()
	return nil
}

// Retire never closes a foreign socket or a newer owner's session.
func (r *Runtime) Retire(endpoint string, proof ingressmanifest.Proof) error {
	if r == nil {
		return ErrInvalid
	}
	canonical, err := canonicalEndpoint(endpoint)
	if err != nil {
		return err
	}
	endpoint = canonical
	r.mu.Lock()
	s, ok := r.sessions[endpoint]
	if !ok || s.proof != proof {
		r.mu.Unlock()
		return ErrUnknown
	}
	if s.retiring {
		r.mu.Unlock()
		return ErrConflict
	}
	s.retiring = true
	s.cancel()
	r.mu.Unlock()
	err = <-s.done
	r.mu.Lock()
	defer r.mu.Unlock()
	if releaseErr := r.registry.Release(endpoint, proof); releaseErr != nil {
		return releaseErr
	}
	delete(r.sessions, endpoint)
	if err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}
	return nil
}
