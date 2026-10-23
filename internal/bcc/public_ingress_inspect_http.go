package bcc

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
)

var errPublicIngressInspectNotFound = errors.New("public ingress inspection not found")

type PublicIngressInspectReceipt struct {
	JobID        string          `json:"job_id"`
	NodeID       string          `json:"node_id"`
	Status       string          `json:"status"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	ReadOnly     bool            `json:"read_only"`
	ReadyToApply bool            `json:"ready_to_apply"`
	Evidence     json.RawMessage `json:"evidence,omitempty"`
}

func (s *Store) PublicIngressInspectReceipt(id string) (PublicIngressInspectReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.st.Jobs[id]
	if !ok || j.Type != agentjob.ActionPublicIngressInspect {
		return PublicIngressInspectReceipt{}, errPublicIngressInspectNotFound
	}
	receipt := PublicIngressInspectReceipt{JobID: j.ID, NodeID: j.NodeID, Status: j.Status, CreatedAt: j.CreatedAt, UpdatedAt: j.UpdatedAt, ReadOnly: true}
	if j.Status == "succeeded" {
		if err := validatePublicIngressInspectSnapshot(j.Output, j.ID, j.NodeID, j.Params); err != nil {
			return PublicIngressInspectReceipt{}, err
		}
		receipt.Evidence = json.RawMessage(j.Output)
	}
	return receipt, nil
}

func decodePublicIngressInspectRequest(dec *json.Decoder) (string, error) {
	start, err := dec.Token()
	if err != nil || start != json.Delim('{') {
		return "", errors.New("inspect request must be an object")
	}
	var tunnelID string
	seen := false
	for dec.More() {
		key, err := dec.Token()
		if err != nil || key != "tunnel_id" || seen {
			return "", errors.New("invalid or duplicate inspect request field")
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return "", errors.New("invalid inspect tunnel")
		}
		if err := json.Unmarshal(raw, &tunnelID); err != nil {
			return "", errors.New("invalid inspect tunnel")
		}
		seen = true
	}
	end, err := dec.Token()
	if err != nil || end != json.Delim('}') || !seen || tunnelID == "" {
		return "", errors.New("missing inspect tunnel")
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return "", errors.New("trailing inspect request data")
	}
	return tunnelID, nil
}

func (s *Server) publicIngressInspectAPI(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		receipt, err := s.store.PublicIngressInspectReceipt(r.URL.Query().Get("id"))
		if errors.Is(err, errPublicIngressInspectNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "stored inspect evidence is invalid", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, receipt)
	case http.MethodPost:
		if validateJobSigningKey(s.jobKey) != nil {
			http.Error(w, "agent job signing is not configured", http.StatusServiceUnavailable)
			return
		}
		s.mutationMu.Lock()
		defer s.mutationMu.Unlock()
		tunnelID, err := decodePublicIngressInspectRequest(json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)))
		if err != nil {
			http.Error(w, "invalid inspect request", http.StatusBadRequest)
			return
		}
		now := s.now().UTC()
		job, err := s.store.CreatePublicIngressInspect(tunnelID, now, AuditEntry{Timestamp: now, Actor: "admin", RemoteIP: s.clientIP(r), Details: withRequest(r, nil)})
		if err != nil {
			s.auditFailure(w, r, "public_ingress.inspect", tunnelID, nil, err, http.StatusBadRequest)
			return
		}
		if err := s.FlushSecurityAuditIntents(); err != nil {
			w.Header().Set("X-BAFT-Audit-State", "pending")
		}
		writeJSON(w, http.StatusAccepted, struct {
			Job          Job  `json:"job"`
			ReadOnly     bool `json:"read_only"`
			ReadyToApply bool `json:"ready_to_apply"`
		}{Job: job, ReadOnly: true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
