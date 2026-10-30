package node

import (
	"context"
	"net/http"
	"sync"
)

// listenerLifecycle owns handlers separately from HTTP connection state.
// HTTP/2 may mark a reset stream idle before its handler has released the
// session's resources; hijacked WebSockets are not owned by Server.Shutdown.
type listenerLifecycle struct {
	ctx    context.Context
	cancel context.CancelFunc
	next   http.Handler
	mu     sync.Mutex
	closed bool
	active sync.WaitGroup
}

func newListenerLifecycle(parent context.Context, next http.Handler) *listenerLifecycle {
	ctx, cancel := context.WithCancel(parent)
	return &listenerLifecycle{ctx: ctx, cancel: cancel, next: next}
}

func (l *listenerLifecycle) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		http.Error(w, "listener is stopping", http.StatusServiceUnavailable)
		return
	}
	// Add and admission closure share the mutex: Wait cannot race a new
	// handler registration after it has observed an empty group.
	l.active.Add(1)
	l.mu.Unlock()
	defer l.active.Done()
	l.next.ServeHTTP(w, r)
}

func (l *listenerLifecycle) close(srv *http.Server) {
	l.mu.Lock()
	l.closed = true
	l.mu.Unlock()
	l.cancel()
	// Cancellation alone does not unblock a read on the carrier. Close HTTP
	// connections before joining; WS handlers separately close hijacked FDs.
	_ = srv.Close()
	l.active.Wait()
}
