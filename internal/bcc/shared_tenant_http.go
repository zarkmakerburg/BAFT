package bcc

import (
	"net/http"
	"strings"
)

func (s *Server) sharedTenantMutationAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.admin(w, r) {
		return
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	var in SharedTenantMutationRequest
	if err := decodeJSON(r, &in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	in.TunnelID = strings.TrimSpace(in.TunnelID)
	in.Operation = strings.TrimSpace(in.Operation)
	in.NodeID = strings.TrimSpace(in.NodeID)
	in.Listen = strings.TrimSpace(in.Listen)
	in.PeerID = strings.TrimSpace(in.PeerID)
	in.RouteID = strings.TrimSpace(in.RouteID)
	in.Target = strings.TrimSpace(in.Target)
	details := map[string]any{
		"operation": in.Operation, "peer_id": in.PeerID, "route_id": in.RouteID,
		// Never include the public key in audit metadata.
	}
	if err := s.auditAdmin(r, "tunnel.shared_tenant.request", in.TunnelID, "requested", details); err != nil {
		http.Error(w, "audit log failure", http.StatusInternalServerError)
		return
	}
	if err := s.store.ReconcileSharedTenantMutationAcks(s.now()); err != nil {
		s.auditFailure(w, r, "tunnel.shared_tenant.reconcile", in.TunnelID, details, err, http.StatusConflict)
		return
	}
	job, err := s.store.CreateSharedTenantMutationJob(in, s.now())
	if err != nil {
		s.auditFailure(w, r, "tunnel.shared_tenant.request", in.TunnelID, details, err, http.StatusConflict)
		return
	}
	if err := s.auditAdmin(r, "tunnel.shared_tenant.queued", in.TunnelID, "success", map[string]any{
		"job_id": job.ID, "operation": in.Operation, "peer_id": in.PeerID, "route_id": in.RouteID,
	}); err != nil {
		// The job is already durably queued; report the audit failure rather
		// than pretending that the operation was not accepted.
		w.Header().Set("X-BAFT-Audit-State", "pending")
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job": job, "message": "tenant mutation queued; existing tunnel remains active until the agent confirms"})
}
