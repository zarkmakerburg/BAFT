package bcc

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
)

var errPublicIngressPrepareNotFound = errors.New("public ingress prepare not found")

type PublicIngressPrepareReceipt struct {
	JobID        string          `json:"job_id"`
	NodeID       string          `json:"node_id"`
	Status       string          `json:"status"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	PreparedOnly bool            `json:"prepared_only"`
	ReadyToApply bool            `json:"ready_to_apply"`
	Evidence     json.RawMessage `json:"evidence,omitempty"`
}

func (s *Store) PublicIngressPrepareReceipt(id string) (PublicIngressPrepareReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.st.Jobs[id]
	if !ok || j.Type != agentjob.ActionPublicIngressPrepare {
		return PublicIngressPrepareReceipt{}, errPublicIngressPrepareNotFound
	}
	receipt := PublicIngressPrepareReceipt{JobID: j.ID, NodeID: j.NodeID, Status: j.Status, CreatedAt: j.CreatedAt, UpdatedAt: j.UpdatedAt, PreparedOnly: true}
	if j.Status == "succeeded" {
		op, ok := s.st.PublicIngressOperations[j.Params["tunnel_id"]]
		if !ok {
			return PublicIngressPrepareReceipt{}, errors.New("prepared operation cursor missing")
		}
		if err := validatePublicIngressPrepareReceipt(j.Output, j, op); err != nil {
			return PublicIngressPrepareReceipt{}, err
		}
		receipt.Evidence = json.RawMessage(j.Output)
	}
	return receipt, nil
}

func (s *Server) publicIngressPrepareAPI(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		receipt, err := s.store.PublicIngressPrepareReceipt(r.URL.Query().Get("id"))
		if errors.Is(err, errPublicIngressPrepareNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "stored ingress prepare evidence is invalid", http.StatusInternalServerError)
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
			http.Error(w, "invalid ingress prepare request", http.StatusBadRequest)
			return
		}
		now := s.now().UTC()
		job, err := s.store.CreatePublicIngressPrepare(tunnelID, now, AuditEntry{Timestamp: now, Actor: "admin", RemoteIP: s.clientIP(r), Details: withRequest(r, nil)})
		if err != nil {
			s.auditFailure(w, r, "public_ingress.prepare", tunnelID, nil, err, http.StatusBadRequest)
			return
		}
		if err := s.FlushSecurityAuditIntents(); err != nil {
			w.Header().Set("X-BAFT-Audit-State", "pending")
		}
		writeJSON(w, http.StatusAccepted, struct {
			Job          Job  `json:"job"`
			PreparedOnly bool `json:"prepared_only"`
			ReadyToApply bool `json:"ready_to_apply"`
		}{Job: job, PreparedOnly: true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
