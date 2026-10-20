package bcc

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
)

// PublicIngressPreflightRequest inspects an existing tunnel's IR loopback
// route. It neither reserves the public port nor authorizes an ingress apply.
type PublicIngressPreflightRequest struct {
	IRNode      string `json:"ir_node"`
	TunnelID    string `json:"tunnel_id"`
	PublicPort  int    `json:"public_port"`
	LocalTarget string `json:"local_target"`
}

// Lost delivery/ACK must not block future read-only inspections forever. This
// is not a retry lease for mutating jobs and conveys no host resource ownership.
const publicIngressPreflightPendingLifetime = 5 * time.Minute

// CreatePublicIngressPreflight atomically persists a read-only job and its
// audit intent. Tunnel ownership is checked against BCC state, never agent JSON.
func (s *Store) CreatePublicIngressPreflight(req PublicIngressPreflightRequest, now time.Time, audit AuditEntry) (Job, error) {
	spec, err := PlanPublicIngress(req.IRNode, req.TunnelID, req.PublicPort, req.LocalTarget)
	if err != nil {
		return Job{}, err
	}
	params := map[string]string{"tunnel_id": spec.TunnelID, "public_port": strconv.Itoa(spec.PublicPort), "local_target": spec.LocalTarget}
	if err := (agentjob.Job{SchemaVersion: agentjob.SchemaVersion, JobID: "preflight-validation", NodeID: spec.IRNode, Action: agentjob.ActionPublicIngressPreflight, Params: params, IssuedAt: now, ExpiresAt: now.Add(time.Minute)}).Validate(); err != nil {
		return Job{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.st.Nodes[spec.IRNode]
	if !ok || n.Revoked || n.AgentTokenHash == "" || (n.Role != "worker" && n.Role != "master") {
		return Job{}, errors.New("preflight requires an active enrolled IR node")
	}
	if n.AgentSeen.IsZero() || now.Sub(n.AgentSeen) > 5*time.Minute || n.AgentSeen.After(now.Add(agentjob.ClockSkew)) {
		return Job{}, errors.New("IR agent is offline or has no recent authenticated contact")
	}
	tunnel, ok := s.st.Tunnels[spec.TunnelID]
	if !ok || tunnel.IRNode != spec.IRNode {
		return Job{}, errors.New("tunnel does not belong to the requested IR node")
	}
	expected, err := PlanPublicIngress(spec.IRNode, spec.TunnelID, spec.PublicPort, tunnel.RouteListen)
	if err != nil || expected.LocalTarget != spec.LocalTarget {
		return Job{}, errors.New("local_target must match the tunnel's IR route listener")
	}
	var expired []string
	for _, old := range s.st.Jobs {
		if old.Type == agentjob.ActionPublicIngressPreflight && old.NodeID == spec.IRNode && (old.Status == "queued" || old.Status == "dispatched") {
			if now.Sub(old.CreatedAt) < publicIngressPreflightPendingLifetime {
				return Job{}, errors.New("an IR preflight is already pending")
			}
			expired = append(expired, old.ID)
		}
	}
	before, err := cloneState(s.st)
	if err != nil {
		return Job{}, err
	}
	for _, id := range expired {
		old := s.st.Jobs[id]
		old.Status, old.Message, old.UpdatedAt = "failed", "read-only preflight expired; a fresh inspection was requested", now.UTC()
		s.st.Jobs[id] = old
	}
	job := s.newJobLocked(Job{Type: agentjob.ActionPublicIngressPreflight, NodeID: spec.IRNode, Params: params})
	job.CreatedAt, job.UpdatedAt = now.UTC(), now.UTC()
	s.st.Jobs[job.ID] = job
	audit.Action, audit.Target, audit.Outcome = "public_ingress.preflight", spec.TunnelID, "success"
	audit.Details = mergeAuditDetails(audit.Details, map[string]any{"job_id": job.ID, "ir_node": spec.IRNode, "public_port": spec.PublicPort, "local_target": spec.LocalTarget, "read_only": true})
	if len(expired) != 0 {
		audit.Details["expired_preflight_job_ids"] = expired
	}
	if _, err := s.enqueueSecurityAuditLocked(audit); err != nil {
		s.st = before
		return Job{}, err
	}
	if err := s.saveLocked(); err != nil {
		s.st = before
		return Job{}, err
	}
	return job, nil
}

func (s *Server) publicIngressPreflightAPI(w http.ResponseWriter, r *http.Request) {
	if !s.admin(w, r) {
		return
	}
	if r.Method == http.MethodGet {
		receipt, err := s.store.PublicIngressPreflightReceipt(r.URL.Query().Get("id"))
		if errors.Is(err, errPublicIngressPreflightNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "stored preflight evidence is invalid", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, receipt)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if len(s.jobKey) != ed25519.PrivateKeySize {
		http.Error(w, "agent job signing is not configured", http.StatusServiceUnavailable)
		return
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	var req PublicIngressPreflightRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "invalid preflight request", http.StatusBadRequest)
		return
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		http.Error(w, "preflight request must contain one JSON object", http.StatusBadRequest)
		return
	}
	now := s.now().UTC()
	job, err := s.store.CreatePublicIngressPreflight(req, now, AuditEntry{Timestamp: now, Actor: "admin", RemoteIP: s.clientIP(r), Details: withRequest(r, nil)})
	if err != nil {
		s.auditFailure(w, r, "public_ingress.preflight", req.TunnelID, nil, err, http.StatusBadRequest)
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
}
