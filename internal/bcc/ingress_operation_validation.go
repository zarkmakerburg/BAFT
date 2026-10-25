package bcc

import (
	"errors"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// Compatibility: historical intents with no reviewed identity remain pending
// and unverified. New command/receipt records must be exact durable matches.
func validatePersistedIngressOperations(st state) error {
	operationIDs := make(map[string]string)
	leaseIDs := make(map[string]string)
	jobIDs := make(map[string]string)
	for id, op := range st.PublicIngressOperations {
		if id != op.Spec.TunnelID || !op.Proof.valid() || op.Status != PublicIngressPending || op.CreatedAt.IsZero() {
			return errors.New("invalid persisted ingress operation")
		}
		reservation, ok := st.PublicIngressReservations[id]
		if !ok || reservation.Spec != op.Spec {
			return errors.New("persisted ingress operation has no reservation")
		}
		tunnel, ok := st.Tunnels[id]
		if !ok || tunnel.IRNode != op.Spec.IRNode || tunnel.RouteListen != op.Spec.LocalTarget || !tunnelKeepsPorts(tunnel) {
			return errors.New("persisted ingress operation tunnel mismatch")
		}
		proof, err := ingressmanifest.Preview(publicIngressManifestSpec(op.Spec, tunnel.InstanceID, op.Proof.Generation))
		if err != nil || proof.ManifestSHA256 != op.Proof.ManifestDigest {
			return errors.New("persisted ingress manifest mismatch")
		}
		if op.ReviewedPlanHash == "" {
			if op.OperationID != "" || op.LeaseID != "" || op.Command != nil || op.Receipt != nil || op.Dispatch != nil || op.Retirement != nil {
				return errors.New("legacy operation cannot contain reviewed command or receipt")
			}
			continue
		}
		if tunnel.Phase != TunnelActive || tunnel.PlanHash != op.ReviewedPlanHash || tunnel.ClientPort != op.Spec.PublicPort {
			return errors.New("reviewed operation changed tunnel plan or client port")
		}
		if (op.OperationID == "") != (op.LeaseID == "") {
			return errors.New("incomplete operation identity")
		}
		if op.OperationID == "" {
			if op.Command != nil || op.Receipt != nil || op.Dispatch != nil || op.Retirement != nil {
				return errors.New("command before operation identity")
			}
			continue // legacy reviewed desired-only record, never dispatched
		}
		if !validIngressCommandID(op.OperationID) || !validIngressCommandID(op.LeaseID) {
			return errors.New("invalid persisted operation or lease identity")
		}
		if existing := operationIDs[op.OperationID]; existing != "" {
			return errors.New("duplicate operation ID")
		}
		operationIDs[op.OperationID] = id
		if existing := leaseIDs[op.LeaseID]; existing != "" {
			return errors.New("duplicate lease ID")
		}
		leaseIDs[op.LeaseID] = id
		if op.Command == nil {
			if op.Receipt != nil || op.Dispatch != nil || op.Retirement != nil {
				return errors.New("receipt or dispatch without command")
			}
			continue
		}
		cmd := *op.Command
		if cmd.Action != "public_ingress_apply" || cmd.Validate(cmd.IssuedAt) != nil || cmd.Binding.OperationID != op.OperationID || cmd.Binding.LeaseID != op.LeaseID || cmd.Binding.PlanSHA256 != op.ReviewedPlanHash || cmd.Binding.NodeID != op.Spec.IRNode || int(cmd.Binding.Port) != op.Spec.PublicPort || int(cmd.Binding.ConfigPort) != op.Spec.PublicPort || cmd.Binding.Spec.TunnelID != id || cmd.Binding.Spec.InstanceID != tunnel.InstanceID || cmd.Binding.Spec.Generation != op.Proof.Generation || cmd.Binding.ListenerID != op.Proof.ListenerID || cmd.Binding.FirewallID != op.Proof.FirewallID || cmd.Binding.Proof != proof {
			return errors.New("staged command differs from persisted reviewed operation")
		}
		if previous := jobIDs[cmd.JobID]; previous != "" {
			return errors.New("duplicate staged command ID")
		}
		jobIDs[cmd.JobID] = id
		if op.Dispatch != nil {
			d := *op.Dispatch
			if (d.Phase != "held" && d.Phase != "queued") || d.JobID != cmd.JobID || d.OperationID != op.OperationID || d.LeaseID != op.LeaseID || d.CreatedAt.IsZero() {
				return errors.New("persisted held dispatch differs from staged reviewed command")
			}
			for _, job := range st.Jobs {
				if job.ID == d.JobID {
					if d.Phase == "held" {
						return errors.New("held ingress job collides with runnable queue")
					}
					if _, err := operationalIngressJobCommand(job, cmd.IssuedAt); err != nil || job.IngressCommand == nil || *job.IngressCommand != cmd {
						return errors.New("queued ingress job differs from durable operation")
					}
				}
			}
		}
		if op.Dispatch != nil && op.Dispatch.Phase == "queued" {
			job, exists := st.Jobs[cmd.JobID]
			if !exists || job.ID != cmd.JobID {
				return errors.New("queued ingress command has no job")
			}
		}
		if op.Retirement != nil {
			if err := validateIngressRetirement(st, op, jobIDs); err != nil {
				return err
			}
		}
		if op.Receipt != nil {
			r := *op.Receipt
			if r.SchemaVersion != 1 || r.JobID != cmd.JobID || r.Action != cmd.Action || r.Binding != cmd.Binding {
				return errors.New("receipt differs from exact persisted command")
			}
			if op.Dispatch != nil && op.Dispatch.Phase == "queued" {
				if err := r.ValidateForCommand(cmd); err != nil {
					return err
				}
			}
		}
	}
	for key, job := range st.Jobs {
		if !isOperationalIngressJob(job) && job.IngressCommand == nil {
			continue
		}
		if key != job.ID || job.IngressCommand == nil {
			return errors.New("invalid operational queue key or command")
		}
		switch job.Status {
		case "queued", "dispatched", "succeeded", "failed":
		default:
			return errors.New("invalid operational queue status")
		}
		op, ok := st.PublicIngressOperations[job.IngressCommand.Binding.Spec.TunnelID]
		command, dispatch, _ := ingressQueuedParts(op, job.Type == "public_ingress_retire")
		if !ok || command == nil || *command != *job.IngressCommand || dispatch == nil || dispatch.Phase != "queued" {
			return errors.New("orphan operational queue record")
		}
	}
	return nil
}

func cloneIngressOperation(op PublicIngressOperation) PublicIngressOperation {
	if op.Retirement != nil {
		r := *op.Retirement
		d := r.Dispatch
		r.Dispatch = d
		if r.Receipt != nil {
			receipt := *r.Receipt
			receipt.FirewallEvidence = append([]ingressauthority.FirewallEvidenceV1(nil), receipt.FirewallEvidence...)
			r.Receipt = &receipt
		}
		op.Retirement = &r
	}
	if op.Command != nil {
		cmd := *op.Command
		op.Command = &cmd
	}
	if op.Dispatch != nil {
		d := *op.Dispatch
		op.Dispatch = &d
	}
	if op.Receipt != nil {
		r := *op.Receipt
		r.FirewallEvidence = append([]ingressauthority.FirewallEvidenceV1(nil), r.FirewallEvidence...)
		op.Receipt = &r
	}
	return op
}

// Return copies so callers cannot mutate the persisted command or receipt.
func (s *Store) GetPublicIngressOperation(tunnelID string) (PublicIngressOperation, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	op, ok := s.st.PublicIngressOperations[tunnelID]
	if !ok {
		return PublicIngressOperation{}, false
	}
	return cloneIngressOperation(op), true
}

func validIngressCommandID(v string) bool {
	if len(v) < 1 || len(v) > 64 {
		return false
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		letter := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		digit := c >= '0' && c <= '9'
		if !letter && !digit && (i == 0 || (c != '.' && c != '_' && c != '-')) {
			return false
		}
	}
	return true
}
