package bcc

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// reviewedIngressReservationRequest binds the one selected client config port
// to an already persisted, active, reviewed tunnel. No Agent apply occurs here.
type reviewedIngressReservationRequest struct {
	TunnelID   string `json:"tunnel_id"`
	PlanHash   string `json:"plan_hash"`
	ClientPort int    `json:"client_port"`
}

func decodeReviewedIngressReservation(dec *json.Decoder) (reviewedIngressReservationRequest, error) {
	var out reviewedIngressReservationRequest
	token, err := dec.Token()
	if err != nil || token != json.Delim('{') {
		return out, errors.New("expected reviewed reservation object")
	}
	fields := map[string]json.RawMessage{}
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return out, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return out, errors.New("invalid reviewed reservation field")
		}
		if _, duplicate := fields[key]; duplicate {
			return out, errors.New("duplicate reviewed reservation field")
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return out, err
		}
		fields[key] = raw
	}
	if _, err := dec.Token(); err != nil {
		return out, err
	}
	if len(fields) != 3 {
		return out, errors.New("unknown or missing reviewed reservation field")
	}
	for _, key := range []string{"tunnel_id", "plan_hash", "client_port"} {
		raw, ok := fields[key]
		if !ok || string(raw) == "null" {
			return out, errors.New("missing reviewed reservation field")
		}
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, err
	}
	if out.TunnelID == "" || out.PlanHash == "" || out.ClientPort < 1 || out.ClientPort > 65535 {
		return out, errors.New("invalid reviewed reservation values")
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return out, errors.New("reviewed reservation requires exactly one object")
	}
	return out, nil
}

func (s *Server) reviewedPublicIngressReservationAPI(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	req, err := decodeReviewedIngressReservation(dec)
	if err != nil {
		http.Error(w, "invalid reviewed ingress reservation request", http.StatusBadRequest)
		return
	}
	tunnel, ok := s.store.GetTunnel(req.TunnelID)
	if !ok {
		http.Error(w, "reviewed tunnel not found", http.StatusBadRequest)
		return
	}
	spec := PublicIngressSpec{IRNode: tunnel.IRNode, TunnelID: req.TunnelID, PublicPort: req.ClientPort, LocalTarget: tunnel.RouteListen}
	now := s.now().UTC()
	saved, err := s.store.ReserveReviewedPublicIngress(spec, req.PlanHash, now, AuditEntry{Timestamp: now, Actor: "admin", RemoteIP: s.clientIP(r), Details: withRequest(r, nil)})
	if err != nil {
		s.auditFailure(w, r, "public_ingress.reserve_reviewed", req.TunnelID, nil, err, http.StatusBadRequest)
		return
	}
	if err := s.FlushSecurityAuditIntents(); err != nil {
		w.Header().Set("X-BAFT-Audit-State", "pending")
	}
	writeJSON(w, http.StatusOK, struct {
		Reservation       PublicIngressReservation `json:"reservation"`
		DesiredOnly       bool                     `json:"desired_only"`
		OwnershipVerified bool                     `json:"ownership_verified"`
		ReadyToApply      bool                     `json:"ready_to_apply"`
	}{Reservation: saved, DesiredOnly: true})
}
