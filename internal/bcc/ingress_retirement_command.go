package bcc

import (
	"errors"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"reflect"
	"time"
)

// IngressRetirement keeps its new signed command identity separately from the
// immutable Apply command and receipt. It is not a replacement resource lease.
type IngressRetirement struct {
	Command  ingressauthority.CommandV1  `json:"command"`
	Dispatch IngressDispatchIntent       `json:"dispatch"`
	Receipt  *ingressauthority.ReceiptV1 `json:"receipt,omitempty"`
}

func ingressQueuedParts(op PublicIngressOperation, retiring bool) (*ingressauthority.CommandV1, *IngressDispatchIntent, *ingressauthority.ReceiptV1) {
	if retiring {
		if op.Retirement == nil {
			return nil, nil, nil
		}
		return &op.Retirement.Command, &op.Retirement.Dispatch, op.Retirement.Receipt
	}
	return op.Command, op.Dispatch, op.Receipt
}

// StageReviewedIngressRetireCommand authorizes neither deletion nor readiness.
// It binds a NEW signed Retire JobID to the original applied resource identity.
func (s *Store) StageReviewedIngressRetireCommand(id string, c ingressauthority.CommandV1, now time.Time) error {
	if err := c.Validate(now); err != nil {
		return err
	}
	if _, err := c.AgentJob(); err != nil {
		return err
	}
	if c.Action != "public_ingress_retire" {
		return errors.New("retirement requires new retire action")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validatePersistedIngressOperations(s.st); err != nil {
		return err
	}
	op, ok := s.st.PublicIngressOperations[id]
	if !ok || op.Command == nil || op.Dispatch == nil || op.Dispatch.Phase != "queued" || op.Receipt == nil || op.Receipt.Phase != "applied" || op.Receipt.ValidateForCommand(*op.Command) != nil || c.Binding != op.Command.Binding || c.JobID == op.Command.JobID {
		return errors.New("retirement requires exact authenticated applied resource")
	}
	if op.Retirement != nil {
		if op.Retirement.Command == c {
			return nil
		}
		return errors.New("retirement command identity is immutable")
	}
	for _, other := range s.st.PublicIngressOperations {
		if other.Command != nil && other.Command.JobID == c.JobID || other.Retirement != nil && other.Retirement.Command.JobID == c.JobID {
			return errors.New("retirement JobID already claimed")
		}
	}
	if _, exists := s.st.Jobs[c.JobID]; exists {
		return errors.New("retirement JobID collides with queue")
	}
	before, err := cloneState(s.st)
	if err != nil {
		return err
	}
	op.Retirement = &IngressRetirement{Command: c, Dispatch: IngressDispatchIntent{JobID: c.JobID, OperationID: op.OperationID, LeaseID: op.LeaseID, Phase: "held", CreatedAt: now.UTC()}}
	s.st.PublicIngressOperations[id] = op
	if _, err := s.enqueueSecurityAuditLocked(AuditEntry{Timestamp: now.UTC(), Actor: "bcc", Action: "public_ingress.retire.stage", Target: id, Outcome: "held", Details: map[string]any{"job_id": c.JobID, "operation_id": op.OperationID, "lease_id": op.LeaseID}}); err != nil {
		s.st = before
		return err
	}
	if err := s.saveLocked(); err != nil {
		s.st = before
		return err
	}
	return nil
}

func (s *Store) enqueueReviewedIngressRetire(id string, now time.Time, audit AuditEntry) (Job, error) {
	return s.enqueueReviewedIngressAction(id, now, audit, true)
}

func validateIngressRetirement(st state, op PublicIngressOperation, jobIDs map[string]string) error {
	r := op.Retirement
	c := r.Command
	if op.Command == nil || op.Dispatch == nil || op.Dispatch.Phase != "queued" || op.Receipt == nil || op.Receipt.Phase != "applied" || op.Receipt.ValidateForCommand(*op.Command) != nil || c.Action != "public_ingress_retire" || c.Binding != op.Command.Binding || c.JobID == op.Command.JobID || c.Validate(c.IssuedAt) != nil {
		return errors.New("invalid retirement history")
	}
	if _, err := c.AgentJob(); err != nil {
		return err
	}
	if jobIDs[c.JobID] != "" {
		return errors.New("duplicate retirement job ID")
	}
	jobIDs[c.JobID] = op.Spec.TunnelID
	d := r.Dispatch
	if d.JobID != c.JobID || d.OperationID != op.OperationID || d.LeaseID != op.LeaseID || d.CreatedAt.IsZero() || (d.Phase != "held" && d.Phase != "queued") {
		return errors.New("invalid retirement dispatch")
	}
	job, exists := st.Jobs[c.JobID]
	if d.Phase == "held" && exists {
		return errors.New("held retirement collides with queue")
	}
	if d.Phase == "queued" {
		if !exists || job.IngressCommand == nil || *job.IngressCommand != c {
			return errors.New("retirement queue missing exact command")
		}
		if _, err := operationalIngressJobCommand(job, c.IssuedAt); err != nil {
			return err
		}
	}
	if r.Receipt != nil {
		if err := r.Receipt.ValidateForCommand(c); err != nil {
			return err
		}
		if r.Receipt.Phase == "retired" && !reflect.DeepEqual(r.Receipt.FirewallEvidence, op.Receipt.FirewallEvidence) {
			return errors.New("retirement evidence changed original rule identity")
		}
	}
	return nil
}
