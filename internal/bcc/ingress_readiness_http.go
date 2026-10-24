package bcc

import (
	"context"
	"net/http"
	"time"
)

// ingressObsTTL caps how often one authenticated dashboard poll can trigger a
// real outbound probe for the same tunnel.
const ingressObsTTL = 10 * time.Second

type ingressObsEntry struct {
	at  time.Time
	obs *IndependentIngressObservation
	res IngressProbeResult
}

func (s *Server) resetIngressObservationCache() {
	s.ingressObsMu.Lock()
	s.ingressObsCache = map[string]ingressObsEntry{}
	s.ingressObsMu.Unlock()
}

// IngressReadinessView is the response of GET /api/ingress/readiness. The
// config card is present only when Ready.
type IngressReadinessView struct {
	IngressReadiness
	Probe *IngressProbeResult `json:"probe,omitempty"`
}

func (s *Server) ingressReadiness(ctx context.Context, tunnelID string) IngressReadinessView {
	now := s.now()
	s.ingressObsMu.Lock()
	e, cached := s.ingressObsCache[tunnelID]
	s.ingressObsMu.Unlock()
	if !cached || now.Sub(e.at) >= ingressObsTTL || e.at.After(now) {
		obs, res, _ := s.ingressObserver.Observe(ctx, s.store, tunnelID, now)
		e = ingressObsEntry{at: now, obs: obs, res: res}
		s.ingressObsMu.Lock()
		s.ingressObsCache[tunnelID] = e
		s.ingressObsMu.Unlock()
	}
	r := s.store.EvaluateIngressReadiness(tunnelID, e.obs, now)
	if !r.Ready {
		r.Card = nil
	}
	view := IngressReadinessView{IngressReadiness: r}
	if e.res.Target != "" || e.res.Failure != "" {
		p := e.res
		view.Probe = &p
	}
	return view
}

// ingressReadinessAPI is admin-authenticated and read-only. It derives
// readiness on demand from the Agent's authenticated receipt plus BCC's own
// probe; nothing is persisted and the card is only emitted for Ready.
func (s *Server) ingressReadinessAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.admin(w, r) {
		return
	}
	id := r.URL.Query().Get("tunnel_id")
	if id == "" || len(id) > 128 {
		http.Error(w, "tunnel_id required", http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, s.ingressReadiness(r.Context(), id))
}
