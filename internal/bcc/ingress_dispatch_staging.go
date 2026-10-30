package bcc

import (
	"errors"
	"time"
)

// IngressDispatchIntent is a durable BCC correlation record, not a queue item.
// An executor never receives it, and held cannot transition to queued here.
type IngressDispatchIntent struct {
	JobID       string    `json:"job_id"`
	OperationID string    `json:"operation_id"`
	LeaseID     string    `json:"lease_id"`
	Phase       string    `json:"phase"`
	CreatedAt   time.Time `json:"created_at"`
}

// StageHeldIngressDispatch persistently binds the exact reviewed command to an
// immutable non-executable intent for recovery and duplicate-retry refusal.
func (s *Store) StageHeldIngressDispatch(tunnelID string, now time.Time) (IngressDispatchIntent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stageHeldIngressDispatchLocked(tunnelID, now, true)
}

func (s *Store) stageHeldIngressDispatchLocked(tunnelID string, now time.Time, persist bool) (IngressDispatchIntent, error) {
	if now.IsZero() {
		return IngressDispatchIntent{}, errors.New("dispatch staging needs a timestamp")
	}
	op, ok := s.st.PublicIngressOperations[tunnelID]
	if !ok || op.Status != PublicIngressPending || op.Command == nil || op.Receipt != nil {
		return IngressDispatchIntent{}, errors.New("no clean staged ingress command available")
	}
	t, ok := s.st.Tunnels[tunnelID]
	if !ok || t.Phase != TunnelActive || t.PlanHash != op.ReviewedPlanHash || t.ClientPort != op.Spec.PublicPort || t.IRNode != op.Spec.IRNode || t.RouteListen != op.Spec.LocalTarget {
		return IngressDispatchIntent{}, errors.New("reviewed tunnel changed")
	}
	cmd := *op.Command
	if cmd.Binding.OperationID != op.OperationID || cmd.Binding.LeaseID != op.LeaseID || cmd.Binding.PlanSHA256 != op.ReviewedPlanHash || cmd.Binding.NodeID != op.Spec.IRNode || cmd.Binding.Spec.TunnelID != tunnelID || cmd.Action != "public_ingress_apply" || cmd.Validate(cmd.IssuedAt) != nil {
		return IngressDispatchIntent{}, errors.New("staged command identity mismatch")
	}
	expected := IngressDispatchIntent{JobID: cmd.JobID, OperationID: op.OperationID, LeaseID: op.LeaseID, Phase: "held"}
	if op.Dispatch != nil {
		old := *op.Dispatch
		if old.JobID == expected.JobID && old.OperationID == expected.OperationID && old.LeaseID == expected.LeaseID && old.Phase == "held" && !old.CreatedAt.IsZero() {
			return old, nil
		}
		return IngressDispatchIntent{}, errors.New("immutable ingress dispatch intent differs")
	}
	for otherID, other := range s.st.PublicIngressOperations {
		if otherID != tunnelID && other.Command != nil && other.Command.JobID == cmd.JobID {
			return IngressDispatchIntent{}, errors.New("staged ingress JobID belongs to another operation")
		}
	}
	for _, job := range s.st.Jobs {
		if job.ID == cmd.JobID {
			return IngressDispatchIntent{}, errors.New("ingress command JobID collides with ordinary BCC queue")
		}
	}
	before, err := cloneState(s.st)
	if err != nil {
		return IngressDispatchIntent{}, err
	}
	expected.CreatedAt = now.UTC()
	op.Dispatch = &expected
	s.st.PublicIngressOperations[tunnelID] = op
	if persist {
		if err := s.saveLocked(); err != nil {
			s.st = before
			return IngressDispatchIntent{}, err
		}
	}
	return expected, nil
}
