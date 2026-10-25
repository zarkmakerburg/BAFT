package bcc

import (
	"errors"
	"io"
	"net/http"
)

func decodeUpdateRequest(r *http.Request, v any) error {
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, updateRequestMaxBytes+1))
	if err != nil {
		return errors.New("unreadable request body")
	}
	if len(body) > updateRequestMaxBytes {
		return errTooLarge
	}
	return strictJSON(body, v)
}

func shortErr(err error) string {
	m := err.Error()
	if len(m) > 200 {
		m = m[:200]
	}
	return m
}

// updates serves GET /api/updates: the verified per-server view. Read-only.
func (s *Server) updates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.admin(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, s.updateEval().view)
}

func (s *Server) updateImportStatus(err error) int {
	switch {
	case errors.Is(err, errTooLarge):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, errUpdateNoRoot):
		return http.StatusServiceUnavailable
	case errors.Is(err, errUpdateRevoked), errors.Is(err, errUpdateOlder), errors.Is(err, errUpdateStale),
		errors.Is(err, errUpdateConflict), errors.Is(err, errUpdateFull):
		return http.StatusConflict
	}
	return http.StatusBadRequest
}

// updateImport serves POST /api/updates/import. It verifies a size-capped,
// operator-supplied bundle against the pinned release root and records it. It
// never fetches anything and never applies anything.
func (s *Server) updateImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.admin(w, r) {
		return
	}
	if r.ContentLength > updateBundleMaxBytes {
		http.Error(w, errTooLarge.Error(), http.StatusRequestEntityTooLarge)
		return
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	refuse := func(err error) {
		s.auditFailure(w, r, "updates.import", "bundle", map[string]any{"reason": shortErr(err)}, err, s.updateImportStatus(err))
	}
	bundle, err := parseUpdateBundle(r.Body)
	if err != nil {
		refuse(err)
		return
	}
	now := s.now()
	rel, rev, err := verifyUpdateBundle(bundle, s.updateRoot(), now)
	if err != nil {
		refuse(err)
		return
	}
	allowRollback := false
	if rel != nil {
		allowRollback = r.Header.Get("X-BAFT-Downgrade-Confirmation") == "DOWNGRADE "+rel.Version+" "+rel.Commit
	}
	res, err := s.store.ImportUpdateRelease(rel, rev, s.updateFloor(), allowRollback, AuditEntry{
		Timestamp: now.UTC(), Actor: "admin", RemoteIP: s.clientIP(r), Details: withRequest(r, nil),
	})
	if err != nil {
		refuse(err)
		return
	}
	if err := s.FlushSecurityAuditIntents(); err != nil {
		w.Header().Set("X-BAFT-Audit-State", "pending")
	}
	status, code := "unchanged", http.StatusOK
	if res.Created || res.RevocationAdded {
		status, code = "imported", http.StatusCreated
	}
	out := map[string]any{"status": status, "revocation_sequence": rev.Sequence, "revocation_updated": res.RevocationAdded}
	if res.Release != nil {
		out["version"], out["commit"], out["digest"] = res.Release.Version, res.Release.Commit, res.Release.Digest
		out["rollback_only"] = res.Release.RollbackOnly
		out["channel"] = updateChannel(res.Release.Version)
	}
	writeJSON(w, code, out)
}

type updatePlanRequest struct {
	NodeID       string `json:"node_id"`
	Kind         string `json:"kind"`
	TargetDigest string `json:"target_digest"`
}

func (s *Server) updatePlanError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errTooLarge):
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
	case errors.Is(err, errUpdateUnknownNode), errors.Is(err, errUpdateUnknownRelease):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		http.Error(w, "invalid request", http.StatusBadRequest)
	}
}

// updatePlan serves POST /api/updates/plan: a deterministic plan for one node.
func (s *Server) updatePlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.admin(w, r) {
		return
	}
	var in updatePlanRequest
	if err := decodeUpdateRequest(r, &in); err != nil {
		s.updatePlanError(w, err)
		return
	}
	plan, err := buildUpdatePlan(s.updateEval(), in.NodeID, in.Kind, in.TargetDigest)
	if err != nil {
		s.updatePlanError(w, err)
		return
	}
	if err := s.auditAdmin(r, "updates.plan", plan.NodeID, "success", map[string]any{
		"kind": plan.Kind, "from": plan.FromVersion, "to": plan.ToVersion, "plan_hash": plan.Hash, "ok": plan.OK,
	}); err != nil {
		http.Error(w, "audit log failure", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

type updateApplyRequest struct {
	NodeID       string `json:"node_id"`
	Kind         string `json:"kind"`
	TargetDigest string `json:"target_digest"`
	PlanHash     string `json:"plan_hash"`
	Confirmation string `json:"confirmation"`
}

// updateApply serves POST /api/updates/apply. It enqueues the EXISTING signed
// update_baft deploy job for exactly one node, only for a plan the administrator
// reviewed (hash must still match) and typed a confirmation for.
func (s *Server) updateApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.admin(w, r) {
		return
	}
	var in updateApplyRequest
	if err := decodeUpdateRequest(r, &in); err != nil {
		s.updatePlanError(w, err)
		return
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	details := map[string]any{"node_id": in.NodeID, "kind": in.Kind, "plan_hash": in.PlanHash}
	deny := func(msg string, status int) {
		s.auditFailure(w, r, "updates.apply", in.NodeID, details, errors.New(msg), status)
	}
	plan, err := buildUpdatePlan(s.updateEval(), in.NodeID, in.Kind, in.TargetDigest)
	if err != nil {
		s.updatePlanError(w, err)
		return
	}
	switch {
	case in.Kind != UpdateKindUpgrade || plan.Executor != UpdateExecAgentJob:
		deny("only upgrades run from BCC; rollback is performed with the verified installer on the server", http.StatusBadRequest)
	case in.PlanHash != plan.Hash:
		deny("plan drifted since it was reviewed; plan again", http.StatusConflict)
	case !plan.OK || plan.Apply == nil:
		deny("plan has failing gates", http.StatusConflict)
	case in.Confirmation != plan.Apply.Confirmation:
		deny("explicit confirmation required", http.StatusBadRequest)
	case s.store.updateDeployInFlight(in.NodeID):
		deny("an update job for this server is already in flight", http.StatusConflict)
	default:
		details["version"], details["target_digest"], details["update_center"] = plan.ToVersion, plan.TargetDigest, true
		jobs, err := s.store.CreateDeployJobs([]string{in.NodeID}, plan.ToVersion, AuditEntry{
			Timestamp: s.now().UTC(), Actor: "admin", RemoteIP: s.clientIP(r), Details: withRequest(r, details),
		})
		if err != nil {
			s.auditFailure(w, r, "updates.apply", in.NodeID, details, err, http.StatusBadRequest)
			return
		}
		if err := s.FlushSecurityAuditIntents(); err != nil {
			w.Header().Set("X-BAFT-Audit-State", "pending")
		}
		writeJSON(w, http.StatusAccepted, jobs)
	}
}
