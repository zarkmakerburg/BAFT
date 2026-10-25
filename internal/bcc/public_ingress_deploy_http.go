package bcc

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

type publicIngressOperationRequest struct {
	TunnelID string `json:"tunnel_id"`
	PlanHash string `json:"plan_hash"`
}

type publicIngressOperationResponse struct {
	TunnelID              string `json:"tunnel_id"`
	OperationID           string `json:"operation_id"`
	JobID                 string `json:"job_id"`
	Action                string `json:"action"`
	DispatchPhase         string `json:"dispatch_phase"`
	ReceiptPhase          string `json:"receipt_phase,omitempty"`
	ReceiptVerified       bool   `json:"receipt_verified"`
	ReadyToApply          bool   `json:"ready_to_apply"`
	ReconciliationPending bool   `json:"reconciliation_pending"`
}

// publicIngressDeployAPI queues only the exact BCC-reviewed signed command.
// Queueing is not host authority: the Agent must independently validate the
// enrolled signature, durable held intent, private lease and every effect.
func (s *Server) publicIngressDeployAPI(w http.ResponseWriter, r *http.Request) {
	s.publicIngressOperationAPI(w, r, false)
}

// publicIngressRetireAPI queues a distinct signed Retire command only after
// the exact Apply command has an authenticated applied receipt.
func (s *Server) publicIngressRetireAPI(w http.ResponseWriter, r *http.Request) {
	s.publicIngressOperationAPI(w, r, true)
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

func newReviewedIngressProof(spec PublicIngressSpec, instanceID string) (PublicIngressExpectedProof, error) {
	manifest := publicIngressManifestSpec(spec, instanceID, 1)
	preview, err := ingressmanifest.Preview(manifest)
	if err != nil {
		return PublicIngressExpectedProof{}, err
	}
	listenerID, err := newIngressIdentity("listener-")
	if err != nil {
		return PublicIngressExpectedProof{}, err
	}
	firewallID, err := newIngressIdentity("firewall-")
	if err != nil {
		return PublicIngressExpectedProof{}, err
	}
	proof := PublicIngressExpectedProof{
		Generation: 1, ManifestDigest: preview.ManifestSHA256,
		ListenerID: listenerID, FirewallID: firewallID,
	}
	if !proof.valid() {
		return PublicIngressExpectedProof{}, errors.New("generated ingress proof is invalid")
	}
	return proof, nil
}

func buildReviewedIngressCommand(op PublicIngressOperation, tunnel Tunnel, now time.Time, action string) (ingressauthority.CommandV1, error) {
	if now.IsZero() || (action != agentjob.ActionPublicIngressApply && action != agentjob.ActionPublicIngressRetire) {
		return ingressauthority.CommandV1{}, errors.New("invalid ingress command request")
	}
	manifest := publicIngressManifestSpec(op.Spec, tunnel.InstanceID, op.Proof.Generation)
	preview, err := ingressmanifest.Preview(manifest)
	if err != nil || preview.ManifestSHA256 != op.Proof.ManifestDigest {
		return ingressauthority.CommandV1{}, errors.New("stored ingress proof does not match canonical manifest")
	}
	jobID, err := newIngressIdentity("job-")
	if err != nil {
		return ingressauthority.CommandV1{}, err
	}
	binding := ingressauthority.BindingV1{
		NodeID: op.Spec.IRNode, OperationID: op.OperationID, LeaseID: op.LeaseID,
		Port: uint16(op.Spec.PublicPort), ConfigPort: uint16(op.Spec.PublicPort),
		ListenEndpoint: manifest.Config.Listen, LocalTarget: op.Spec.LocalTarget,
		Transport: "dedicated", PlanSHA256: op.ReviewedPlanHash,
		ListenerID: op.Proof.ListenerID, FirewallID: op.Proof.FirewallID,
		Spec: manifest, Proof: preview,
	}
	command := ingressauthority.CommandV1{
		IngressProtocol: ingressauthority.ProtocolV1, JobID: jobID, Action: action,
		IssuedAt:  now.UTC().Truncate(time.Second),
		ExpiresAt: now.UTC().Truncate(time.Second).Add(3 * time.Minute),
		Binding:   binding,
	}
	if err := command.Validate(now); err != nil {
		return ingressauthority.CommandV1{}, err
	}
	if _, err := command.AgentJob(); err != nil {
		return ingressauthority.CommandV1{}, err
	}
	return command, nil
}

func (s *Server) publicIngressOperationAPI(w http.ResponseWriter, r *http.Request, retiring bool) {
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
	if !ok || tunnel.Phase != TunnelActive || tunnel.PlanHash != req.PlanHash ||
		tunnel.ClientPort < 1 || tunnel.ClientPort > 65535 || tunnel.RouteListen == "" {
		http.Error(w, "reviewed active tunnel or plan hash does not match", http.StatusConflict)
		return
	}
	spec, err := PlanPublicIngress(tunnel.IRNode, tunnel.ID, tunnel.ClientPort, tunnel.RouteListen)
	if err != nil {
		http.Error(w, "active tunnel has no canonical public ingress plan", http.StatusConflict)
		return
	}
	now := s.now().UTC().Truncate(time.Second)
	audit := AuditEntry{Timestamp: now, Actor: "admin", RemoteIP: s.clientIP(r),
		Details: withRequest(r, map[string]any{"plan_hash": req.PlanHash, "public_port": spec.PublicPort})}

	op, exists := s.store.GetPublicIngressOperation(req.TunnelID)
	if retiring {
		if !exists || op.Spec != spec || op.ReviewedPlanHash != req.PlanHash || op.Command == nil ||
			op.Receipt == nil || op.Receipt.Phase != "applied" || op.Receipt.ValidateForCommand(*op.Command) != nil {
			http.Error(w, "retirement requires the exact reviewed operation and authenticated applied receipt", http.StatusConflict)
			return
		}
		if op.Retirement == nil {
			command, buildErr := buildReviewedIngressCommand(op, tunnel, now, agentjob.ActionPublicIngressRetire)
			if buildErr != nil {
				http.Error(w, "could not construct canonical retirement command", http.StatusConflict)
				return
			}
			if err := s.store.StageReviewedIngressRetireCommand(req.TunnelID, command, now); err != nil {
				http.Error(w, "retirement staging refused", http.StatusConflict)
				return
			}
			op, _ = s.store.GetPublicIngressOperation(req.TunnelID)
		}
		if op.Retirement == nil {
			http.Error(w, "retirement command was not durably staged", http.StatusConflict)
			return
		}
		if op.Retirement.Receipt == nil {
			if _, err := s.store.enqueueReviewedIngressRetire(req.TunnelID, now, audit); err != nil {
				s.auditFailure(w, r, "public_ingress.retire.enqueue", req.TunnelID, map[string]any{"plan_hash": req.PlanHash}, err, http.StatusConflict)
				return
			}
		}
		op, _ = s.store.GetPublicIngressOperation(req.TunnelID)
		response := publicIngressOperationResponse{TunnelID: req.TunnelID, OperationID: op.OperationID,
			JobID: op.Retirement.Command.JobID, Action: agentjob.ActionPublicIngressRetire,
			DispatchPhase: op.Retirement.Dispatch.Phase, ReadyToApply: false, ReconciliationPending: true}
		if receipt := op.Retirement.Receipt; receipt != nil {
			response.ReceiptPhase = receipt.Phase
			response.ReceiptVerified = receipt.ValidateForCommand(op.Retirement.Command) == nil
			response.ReconciliationPending = !response.ReceiptVerified
		}
		if err := s.FlushSecurityAuditIntents(); err != nil {
			w.Header().Set("X-BAFT-Audit-State", "pending")
		}
		writeJSON(w, http.StatusAccepted, response)
		return
	}

	if exists {
		if op.Spec != spec || op.ReviewedPlanHash != req.PlanHash || op.Status != PublicIngressPending || op.Retirement != nil {
			http.Error(w, "an immutable ingress operation already owns this tunnel", http.StatusConflict)
			return
		}
	} else {
		proof, proofErr := newReviewedIngressProof(spec, tunnel.InstanceID)
		if proofErr != nil {
			http.Error(w, "could not construct canonical ingress proof", http.StatusInternalServerError)
			return
		}
		op, err = s.store.RecordReviewedPublicIngressOperation(spec, req.PlanHash, proof, now, audit)
		if err != nil {
			// A concurrent request may have durably created the exact operation.
			if existingOp, found := s.store.GetPublicIngressOperation(req.TunnelID); found &&
				existingOp.Spec == spec && existingOp.ReviewedPlanHash == req.PlanHash && existingOp.Status == PublicIngressPending {
				op, exists = existingOp, true
			} else {
				s.auditFailure(w, r, "public_ingress.operation.reviewed_intent", req.TunnelID, map[string]any{"plan_hash": req.PlanHash}, err, http.StatusConflict)
				return
			}
		} else {
			exists = true
		}
	}
	if op.Command == nil {
		command, buildErr := buildReviewedIngressCommand(op, tunnel, now, agentjob.ActionPublicIngressApply)
		if buildErr != nil {
			http.Error(w, "could not construct canonical apply command", http.StatusConflict)
			return
		}
		if err := s.store.StageReviewedIngressCommand(req.TunnelID, command, now); err != nil {
			http.Error(w, "apply command staging refused", http.StatusConflict)
			return
		}
		op, _ = s.store.GetPublicIngressOperation(req.TunnelID)
	}
	if op.Command == nil {
		http.Error(w, "apply command was not durably staged", http.StatusConflict)
		return
	}
	if op.Dispatch == nil {
		if _, err := s.store.StageHeldIngressDispatch(req.TunnelID, now); err != nil {
			http.Error(w, "dispatch intent staging refused", http.StatusConflict)
			return
		}
		op, _ = s.store.GetPublicIngressOperation(req.TunnelID)
	}
	if op.Dispatch == nil {
		http.Error(w, "dispatch intent was not durably staged", http.StatusConflict)
		return
	}
	if op.Receipt == nil {
		if _, err := s.store.enqueueReviewedIngress(req.TunnelID, now, audit); err != nil {
			s.auditFailure(w, r, "public_ingress.operation.enqueue", req.TunnelID, map[string]any{"plan_hash": req.PlanHash}, err, http.StatusConflict)
			return
		}
	}
	op, _ = s.store.GetPublicIngressOperation(req.TunnelID)
	response := publicIngressOperationResponse{TunnelID: req.TunnelID, OperationID: op.OperationID,
		JobID: op.Command.JobID, Action: agentjob.ActionPublicIngressApply,
		DispatchPhase: op.Dispatch.Phase, ReadyToApply: false, ReconciliationPending: true}
	if receipt := op.Receipt; receipt != nil {
		response.ReceiptPhase = receipt.Phase
		response.ReceiptVerified = receipt.ValidateForCommand(*op.Command) == nil
		response.ReconciliationPending = !response.ReceiptVerified
	}
	if err := s.FlushSecurityAuditIntents(); err != nil {
		w.Header().Set("X-BAFT-Audit-State", "pending")
	}
	writeJSON(w, http.StatusAccepted, response)
}
