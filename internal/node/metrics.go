package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	baftmetrics "github.com/zarkmakerburg/baft/internal/metrics"
	"github.com/zarkmakerburg/baft/internal/session"
	"github.com/zarkmakerburg/baft/internal/verification"
)

func (r *Runtime) startMetrics(ctx context.Context, addr string) (<-chan error, func(), error) {
	ln, err := r.takeEndpointListenerForTest(EndpointMetrics, "metrics", addr)
	if err != nil {
		return nil, nil, err
	}
	if ln == nil {
		ln, err = r.takeMetricsListenerForTest(addr)
		if err != nil {
			return nil, nil, err
		}
	}
	if ln == nil {
		ln, err = net.Listen("tcp", addr)
		if err != nil {
			return nil, nil, fmt.Errorf("metrics listen %s: %w", addr, err)
		}
	}
	srv := &http.Server{
		Handler:           r.managementMetricsHandler(),
		ReadHeaderTimeout: 5 * time.Second,
		MaxHeaderBytes:    8 << 10,
	}
	done := make(chan error, 1)
	go func() {
		err := srv.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		done <- err
	}()
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)
		})
	}
	go func() { <-ctx.Done(); stop() }()
	return done, stop, nil
}

func (r *Runtime) observeDelivery(n uint64) {
	if n == 0 {
		return
	}
	r.deliveredBytes.Add(n)
	r.lastDeliveryUnix.Store(time.Now().UnixNano())
}

func (r *Runtime) verificationSnapshot() verification.RuntimeReport {
	r.verificationMu.RLock()
	digest, supported := r.verificationDigest, r.verificationSupported
	r.verificationMu.RUnlock()
	r.peerMu.Lock()
	registered := make([]*session.Peer, 0, len(r.peers))
	for p := range r.peers {
		registered = append(registered, p)
	}
	r.peerMu.Unlock()
	peers := 0
	for _, p := range registered {
		if !p.ApplicationReady() {
			peers = 0
			break
		}
		peers++
	}
	var last time.Time
	if n := r.lastDeliveryUnix.Load(); n > 0 {
		last = time.Unix(0, n).UTC()
	}
	return verification.RuntimeReport{Version: verification.Version, ConfigDigest: digest, BootID: r.bootID, Supported: supported, GeneratedAt: time.Now().UTC(),
		AuthenticatedPeers: peers, ReceivedBytes: r.ingressBytes.Load(), DeliveredBytes: r.deliveredBytes.Load(), LastDeliveredAt: last}
}

// This additional read-only endpoint stays on the already loopback-only
// management listener. It exports digests/counts, never peers, route names,
// addresses, keys, or application payload.
func (r *Runtime) managementMetricsHandler() http.Handler {
	metrics := baftmetrics.Handler(r.metricsSnapshot)
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/verification" {
			metrics.ServeHTTP(w, req)
			return
		}
		if req.Method != http.MethodGet {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(r.verificationSnapshot())
	})
}
