package bcc

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/zarkmakerburg/baft/internal/verification"
)

func verificationPhase(phase string) bool {
	return phase == TunnelDataIR || phase == TunnelDataEX || phase == TunnelSealingIR || phase == TunnelSealingEX
}

func (s *Store) failVerifiedLocked(t *Tunnel, reason string, now time.Time) {
	if !t.FinalizationBegan {
		s.failLocked(t, reason, now)
		return
	}
	t.Phase, t.Error, t.UpdatedAt = TunnelFinalizeIncomplete, reason, now
}

func proofScope(t Tunnel, node string, p verification.Proof) error {
	role := "ex"
	if node == t.IRNode {
		role = "ir"
	}
	if p.Version != verification.Version || p.TunnelID != t.ID || p.InstanceID != t.InstanceID || p.Role != role || t.ObservedGen[node] <= 0 || p.Generation != t.ObservedGen[node] {
		return errors.New("data proof belongs to another change, instance, role or generation")
	}
	return nil
}

func verifyDeliveryProof(t Tunnel, node string, p verification.Proof) error {
	if err := proofScope(t, node, p); err != nil {
		return err
	}
	d, ok := t.Digests[node]
	boot, err := hex.DecodeString(p.Runtime.BootID)
	digest, digestErr := hex.DecodeString(p.Runtime.ConfigDigest)
	if !ok || p.ConfigSHA256 != d.Config || p.UnitSHA256 != d.Unit || p.MarkerSHA256 != d.Marker ||
		p.Runtime.Version != verification.Version || !p.Runtime.Supported || digestErr != nil || len(digest) != 32 || err != nil || len(boot) != 16 ||
		p.Runtime.AuthenticatedPeers < 1 || p.Runtime.ReceivedBytes == 0 || p.Runtime.DeliveredBytes == 0 || p.Runtime.DeliveredBytes > p.Runtime.ReceivedBytes ||
		p.At.IsZero() || p.Runtime.GeneratedAt.IsZero() || p.Runtime.GeneratedAt.After(p.At) || p.At.Sub(p.Runtime.GeneratedAt) > 5*time.Second ||
		p.Runtime.LastDeliveredAt.IsZero() || p.Runtime.LastDeliveredAt.After(p.Runtime.GeneratedAt) || p.DataAgeMS < 0 || p.DataAgeMS >= int64(verification.FreshFor/time.Millisecond) ||
		p.DataAgeMS != p.At.Sub(p.Runtime.LastDeliveredAt).Milliseconds() || time.Duration(p.DataAgeMS)*time.Millisecond >= verification.FreshFor ||
		(node == t.EXNode && !p.TargetReachable) {
		return errors.New("runtime delivery proof is incomplete, stale or differs from verified artifacts")
	}
	return nil
}

func (s *Store) advanceVerificationLocked(t *Tunnel, now time.Time) *TunnelEvent {
	if t.VerificationStarted.IsZero() {
		t.VerificationStarted = now
	}
	if now.Sub(t.VerificationStarted) >= tunnelStepTimeout {
		s.failVerifiedLocked(t, "real data verification timed out; no healthy result was claimed", now)
		return nil
	}
	node, action := t.IRNode, JobTunnelData
	if t.Phase == TunnelDataEX || t.Phase == TunnelSealingEX {
		node = t.EXNode
	}
	if t.Phase == TunnelSealingIR || t.Phase == TunnelSealingEX {
		action = JobTunnelSeal
	}
	if t.JobID == "" {
		if now.Before(t.VerificationRetryAt) {
			return nil
		}
		s.tunnelJobLocked(t, node, action, nil, now)
		return nil
	}
	j, ok := s.st.Jobs[t.JobID]
	if !ok {
		s.failVerifiedLocked(t, "verification job missing", now)
		return nil
	}
	if j.NodeID != node || j.Type != action {
		s.failVerifiedLocked(t, "verification job belongs to another node or action", now)
		return nil
	}
	if j.Status == "queued" || j.Status == "dispatched" {
		return nil
	}
	if j.Status != "succeeded" {
		s.failVerifiedLocked(t, "verification failed on "+node+": "+j.Message, now)
		return nil
	}
	var p verification.Proof
	if json.Unmarshal([]byte(j.Message), &p) != nil || proofScope(*t, node, p) != nil {
		s.failVerifiedLocked(t, "unreadable or wrongly scoped verification proof from "+node, now)
		return nil
	}
	step := "data"
	if action == JobTunnelSeal {
		step = "seal"
	}
	if p.State == verification.Pass {
		if err := verifyDeliveryProof(*t, node, p); err != nil {
			s.failVerifiedLocked(t, err.Error(), now)
			return nil
		}
		if j.CreatedAt.IsZero() || j.CreatedAt.After(now) || now.Sub(j.CreatedAt) >= verification.FreshFor || p.At.After(now) || now.Sub(p.At)+time.Duration(p.DataAgeMS)*time.Millisecond >= verification.FreshFor {
			p.State, p.Reason = verification.NotAssessed, "verification job outlived its fresh evidence window"
		}
	}
	b, _ := json.Marshal(p)
	t.Evidence = append(t.Evidence, TunnelEvidence{Step: step, Node: node, At: now, OK: p.State == verification.Pass, Detail: string(b)})
	if p.State == verification.NotAssessed {
		t.JobID, t.VerificationRetryAt, t.UpdatedAt = "", now.Add(10*time.Second), now
		return nil
	}
	if p.State != verification.Pass || (action == JobTunnelSeal && !p.ReadyToFinalize) {
		s.failVerifiedLocked(t, "data verification did not pass on "+node+": "+p.Reason, now)
		return nil
	}
	switch t.Phase {
	case TunnelDataIR:
		t.Phase = TunnelDataEX
		s.tunnelJobLocked(t, t.EXNode, JobTunnelData, nil, now)
	case TunnelDataEX:
		t.Phase = TunnelSealingIR
		s.tunnelJobLocked(t, t.IRNode, JobTunnelSeal, nil, now)
	case TunnelSealingIR:
		t.Phase = TunnelSealingEX
		s.tunnelJobLocked(t, t.EXNode, JobTunnelSeal, nil, now)
	case TunnelSealingEX:
		irProof, _ := doctorDeliveryProof(*t, t.IRNode, now)
		if irProof == nil || !irProof.ReadyToFinalize {
			s.failVerifiedLocked(t, "IR sealing proof expired before both nodes were ready", now)
			return nil
		}
		t.Phase, t.FinalizationBegan = TunnelFinalizingIR, true
		s.tunnelJobLocked(t, t.IRNode, JobTunnelFinalize, nil, now)
	}
	return nil
}

// RetryFinalization rechecks both nodes. Cleanup which already began is never
// mislabeled as a rollbackable failure or silently declared active.
func (s *Store) RetryFinalization(id string, now time.Time) (Tunnel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.st.Tunnels[id]
	if !ok || t.Phase != TunnelFinalizeIncomplete {
		return Tunnel{}, errors.New("only incomplete finalization can be retried")
	}
	if j, ok := s.st.Jobs[t.JobID]; ok && (j.Status == "queued" || j.Status == "dispatched") {
		return Tunnel{}, errors.New("previous cleanup job still has no definitive result")
	}
	if err := requireTunnelVerification(s.st.Nodes[t.EXNode], s.st.Nodes[t.IRNode], now); err != nil {
		return Tunnel{}, err
	}
	before, err := cloneState(s.st)
	if err != nil {
		return Tunnel{}, err
	}
	t.Phase, t.Error, t.VerificationStarted = TunnelDataIR, "", now
	s.tunnelJobLocked(&t, t.IRNode, JobTunnelData, nil, now)
	s.st.Tunnels[id] = t
	if err := s.saveLocked(); err != nil {
		s.st = before
		return Tunnel{}, fmt.Errorf("retry could not be persisted: %w", err)
	}
	return t, nil
}

func (s *Server) retryTunnelFinalize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.admin(w, r) {
		return
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	t, err := s.store.RetryFinalization(r.URL.Query().Get("id"), s.now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	_, _ = s.audit.Append(AuditEntry{Timestamp: s.now().UTC(), Actor: "admin", RemoteIP: s.clientIP(r), Action: "tunnel.retry_finalization", Target: t.ID, Outcome: "success"})
	writeJSON(w, http.StatusAccepted, t)
}
