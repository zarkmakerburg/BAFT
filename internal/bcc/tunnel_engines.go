package bcc

import (
	"github.com/zarkmakerburg/baft/internal/tunnelengine"
	"net/http"
)

// tunnelEnginesAPI exposes the same fail-closed driver admission catalog that
// backend plan/create validates. It is read-only; no script installation,
// external downloads or node mutation occurs on GET.
func (s *Server) tunnelEnginesAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.admin(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, tunnelengine.Available())
}
