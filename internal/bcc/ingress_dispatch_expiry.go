package bcc

import (
	"errors"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"time"
)

func (s *Store) recordedOperationalOutcomeLocked(j Job) (*ingressauthority.ReceiptV1, error) {
	if j.IngressCommand == nil {
		return nil, errors.New("missing ingress queue command")
	}
	op, ok := s.st.PublicIngressOperations[j.IngressCommand.Binding.Spec.TunnelID]
	c, d, r := ingressQueuedParts(op, j.Type == "public_ingress_retire")
	if !ok || c == nil || *c != *j.IngressCommand || d == nil || d.Phase != "queued" || r == nil || r.ValidateForCommand(*c) != nil || !r.OwnershipVerified || (r.Phase != "applied" && r.Phase != "retired") {
		return nil, errors.New("no exact authenticated operational success receipt")
	}
	return r, nil
}

// Caller owns s.mu and rolls back the whole transaction on an error. Expiry
// fails only this job; it never frees an uncertain host reservation or listener.
func (s *Store) expireOperationalIngressLocked(j Job, now time.Time) (bool, error) {
	if !isOperationalIngressJob(j) {
		return false, nil
	}
	if j.IngressCommand == nil {
		return false, errors.New("operational job lacks immutable command")
	}
	if now.Before(j.IngressCommand.ExpiresAt) {
		return false, nil
	}
	// A durable receipt admitted before expiry may still need its generic ACK.
	if _, err := s.recordedOperationalOutcomeLocked(j); err == nil {
		return false, nil
	}
	j.Status = "failed"
	j.Message = "operational ingress command expired; resource reconciliation remains required"
	j.UpdatedAt = now
	s.st.Jobs[j.ID] = j
	if _, err := s.enqueueSecurityAuditLocked(AuditEntry{Timestamp: now, Actor: "bcc", Action: "public_ingress.dispatch.expire", Target: j.Params["tunnel_id"], Outcome: "expired", Details: map[string]any{"job_id": j.ID, "ir_node": j.NodeID}}); err != nil {
		return false, err
	}
	return true, nil
}
