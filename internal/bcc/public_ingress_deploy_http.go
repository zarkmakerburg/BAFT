package bcc

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
)

// The reviewed ingress deploy/retire API is a thin, admin-only trigger over the
// store's own fail-closed machinery. It never mints authority and never claims
// readiness: staging uses the same immutable reviewed command as activation
// glue, enqueue stays behind the capability-derived gate, and Ready exists only
// on GET /api/ingress/readiness after independent observation.

type publicIngressOperationRequest struct {
	TunnelID string `json:"tunnel_id"`
	PlanHash string `json:"plan_hash"`
}

type publicIngressOperationResponse struct {
	TunnelID              string `json:"tunnel_id"`
	OperationID           string `json:"operation_id,omitempty"`
	JobID                 string `json:"job_id,omitempty"`
	Action                string `json:"action,omitempty"`
	State                 string `json:"state"`
	Reason                string `json:"reason,omitempty"`
	DispatchPhase         string `json:"dispatch_phase,omitempty"`
	ReceiptPhase          string `json:"receipt_phase,omitempty"`
	ReceiptVerified       bool   `json:"receipt_verified"`
	ReadyToApply          bool   `json:"ready_to_apply"`
	ReconciliationPending bool   `json:"reconciliation_pending"`
	OperationStatus       string `json:"operation_status,omitempty"`
	PublicPort            int    `json:"public_port,omitempty"`
	PlanHash              string `json:"plan_hash,omitempty"`
}

func decodePublicIngressOperationRequest(w http.ResponseWriter, r *http.Request) (publicIngressOperationRequest, error) {
	var out publicIngressOperationRequest
	if r == nil || r.Body == nil {
		return out, errors.New("request body is required")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	dec := json.NewDecoder(r.Body)
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return out, errors.New("request must be a JSON object")
	}
	seen := map[string]bool{}
	for dec.More() {
		tok, err = dec.Token()
		if err != nil {
			return out, errors.New("invalid request key")
		}
		key, ok := tok.(string)
		if !ok || seen[key] || (key != "tunnel_id" && key != "plan_hash") {
			return out, errors.New("duplicate or unknown request field")
		}
		seen[key] = true
		switch key {
		case "tunnel_id":
			if err := dec.Decode(&out.TunnelID); err != nil {
				return out, errors.New("tunnel_id must be a string")
			}
		case "plan_hash":
			if err := dec.Decode(&out.PlanHash); err != nil {
				return out, errors.New("plan_hash must be a string")
			}
		}
	}
	if tok, err = dec.Token(); err != nil || tok != json.Delim('}') {
		return out, errors.New("unterminated request object")
	}
	if len(seen) != 2 || strings.TrimSpace(out.TunnelID) == "" || len(out.TunnelID) > 128 ||
		strings.TrimSpace(out.TunnelID) != out.TunnelID || !ingressDigestPattern.MatchString(out.PlanHash) {
		return out, errors.New("tunnel_id and a lowercase SHA-256 plan_hash are required")
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		return out, errors.New("request must contain exactly one JSON object")
	}
	return out, nil
}

// StageReviewedIngressForPlan stages (idempotently) the reviewed operation,
// immutable Apply command and HELD dispatch for an ACTIVE tunnel whose current
// plan hash equals planHash, and saves. It never creates a Job.
func (s *Store) StageReviewedIngressForPlan(tunnelID, planHash string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.st.Tunnels[tunnelID]
	if !ok || t.Phase != TunnelActive || t.PlanHash == "" || t.PlanHash != planHash {
		return errors.New("reviewed active tunnel or plan hash does not match")
	}
	if op, exists := s.st.PublicIngressOperations[tunnelID]; exists && op.ReviewedPlanHash != planHash {
		return errors.New("an immutable ingress operation for a different plan owns this tunnel")
	}
	before, err := cloneState(s.st)
	if err != nil {
		return err
	}
	if err := s.stageReviewedIngressLocked(tunnelID, now); err != nil {
		return err
	}
	if err := s.saveLocked(); err != nil {
		s.st = before
		return err
	}
	return nil
}

func (s *Store) publicIngressOperationView(tunnelID string, now time.Time) (publicIngressOperationResponse, bool) {
	// obs is nil: this endpoint can never report Ready.
	st := s.IngressOperationalState(tunnelID, nil, now)
	if st == nil {
		return publicIngressOperationResponse{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	op, ok := s.st.PublicIngressOperations[tunnelID]
	if !ok || op.Command == nil || op.Dispatch == nil {
		return publicIngressOperationResponse{}, false
	}
	out := publicIngressOperationResponse{
		TunnelID: tunnelID, OperationID: op.OperationID, JobID: st.JobID, Action: op.Command.Action,
		State: st.State, Reason: st.Reason, DispatchPhase: op.Dispatch.Phase, ReadyToApply: false,
		ReconciliationPending: true, OperationStatus: op.Status, PublicPort: op.Spec.PublicPort, PlanHash: op.ReviewedPlanHash,
	}
	// A receipt is only stored after the authenticated receipt endpoint
	// matched it to the exact signed command, so its presence is the verified
	// BCC-side fact; it is still not independent readiness.
	receipt := op.Receipt
	if op.RetireCommand != nil {
		out.Action = op.RetireCommand.Action
		receipt = op.RetireReceipt
	}
	if receipt != nil {
		out.ReceiptPhase = receipt.Phase
		out.ReceiptVerified = true
		out.ReconciliationPending = receipt.Phase != "applied" && receipt.Phase != "retired"
	}
	return out, true
}

func (s *Server) publicIngressDeployAPI(w http.ResponseWriter, r *http.Request) {
	s.publicIngressOperationAPI(w, r, false)
}

func (s *Server) publicIngressRetireAPI(w http.ResponseWriter, r *http.Request) {
	s.publicIngressOperationAPI(w, r, true)
}

func (s *Server) publicIngressOperationAPI(w http.ResponseWriter, r *http.Request, retiring bool) {
	if r.Method == http.MethodGet {
		if !s.admin(w, r) {
			return
		}
		id := strings.TrimSpace(r.URL.Query().Get("tunnel_id"))
		if id == "" || len(id) > 128 {
			http.Error(w, "tunnel_id is required", http.StatusBadRequest)
			return
		}
		view, ok := s.store.publicIngressOperationView(id, s.now())
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, view)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.admin(w, r) {
		return
	}
	if err := validateJobSigningKey(s.jobKey); err != nil {
		http.Error(w, "operational ingress signing is not configured", http.StatusServiceUnavailable)
		return
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	req, err := decodePublicIngressOperationRequest(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	tunnel, ok := s.store.GetTunnel(req.TunnelID)
	if !ok || tunnel.Phase != TunnelActive || tunnel.PlanHash != req.PlanHash {
		http.Error(w, "reviewed active tunnel or plan hash does not match", http.StatusConflict)
		return
	}
	now := s.now().UTC().Truncate(time.Second)
	details := map[string]any{"plan_hash": req.PlanHash, "public_port": tunnel.ClientPort}
	if retiring {
		view, found := s.store.publicIngressOperationView(req.TunnelID, now)
		if !found || view.PlanHash != req.PlanHash {
			http.Error(w, "retirement requires the exact reviewed operation and authenticated applied receipt", http.StatusConflict)
			return
		}
		if err := s.store.EnqueueOperationalIngressRetire(req.TunnelID); err != nil {
			s.auditFailure(w, r, "public_ingress.retire.enqueue", req.TunnelID, details, err, http.StatusConflict)
			return
		}
		view, _ = s.store.publicIngressOperationView(req.TunnelID, now)
		view.Action = agentjob.ActionPublicIngressRetire
		if err := s.FlushSecurityAuditIntents(); err != nil {
			w.Header().Set("X-BAFT-Audit-State", "pending")
		}
		writeJSON(w, http.StatusAccepted, view)
		return
	}
	if err := s.store.StageReviewedIngressForPlan(req.TunnelID, req.PlanHash, now); err != nil {
		s.auditFailure(w, r, "public_ingress.operation.stage", req.TunnelID, details, err, http.StatusConflict)
		return
	}
	// The gate may keep the dispatch HELD (agent lacks the capability): that is
	// reported honestly as state "held", not as an error and never as success.
	if err := s.store.EnqueueReviewedOperationalIngress(req.TunnelID); err != nil && !errors.Is(err, ErrIngressAuthorityUnavailable) {
		view, found := s.store.publicIngressOperationView(req.TunnelID, now)
		if !found || view.DispatchPhase == ingressPhaseHeld {
			s.auditFailure(w, r, "public_ingress.operation.enqueue", req.TunnelID, details, err, http.StatusConflict)
			return
		}
	}
	view, found := s.store.publicIngressOperationView(req.TunnelID, now)
	if !found {
		http.Error(w, "apply command was not durably staged", http.StatusConflict)
		return
	}
	if err := s.FlushSecurityAuditIntents(); err != nil {
		w.Header().Set("X-BAFT-Audit-State", "pending")
	}
	writeJSON(w, http.StatusAccepted, view)
}
