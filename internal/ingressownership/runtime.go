package ingressownership

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"github.com/zarkmakerburg/baft/internal/publicingress"
)

// Runtime owns only sockets it bound itself. This lab-only executor never
// changes host firewall policy and must not be wired to signed deploy jobs.
type Runtime struct {
	mu       sync.Mutex
	registry *Registry
	sessions map[string]*session
}
type session struct {
	proof  ingressmanifest.Proof
	cancel context.CancelFunc
	done   chan error
}

func NewRuntime() *Runtime { return &Runtime{registry: New(), sessions: make(map[string]*session)} }

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
	f, err := publicingress.New(publicingress.Config{Listen: spec.Config.Listen, Target: spec.Config.Target, MaxClients: spec.Config.MaxClients, DialTimeout: time.Duration(spec.Config.DialTimeoutMS) * time.Millisecond})
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.sessions[spec.Config.Listen]; ok {
		if s.proof == proof {
			return nil
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
	if err := parent.Err(); err != nil {
		_ = ln.Close()
		_ = r.registry.Release(spec.Config.Listen, proof)
		return err
	}
	ctx, cancel := context.WithCancel(parent)
	s := &session{proof: proof, cancel: cancel, done: make(chan error, 1)}
	r.sessions[spec.Config.Listen] = s
	go func() { s.done <- f.Serve(ctx, ln); close(s.done) }()
	return nil
}

// Retire never closes a foreign socket or a newer owner's session.
func (r *Runtime) Retire(endpoint string, proof ingressmanifest.Proof) error {
	if r == nil {
		return ErrInvalid
	}
	r.mu.Lock()
	s, ok := r.sessions[endpoint]
	if !ok || s.proof != proof {
		r.mu.Unlock()
		return ErrUnknown
	}
	delete(r.sessions, endpoint)
	s.cancel()
	r.mu.Unlock()
	err := <-s.done
	if releaseErr := r.registry.Release(endpoint, proof); releaseErr != nil {
		return releaseErr
	}
	if err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}
	return nil
}
