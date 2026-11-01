package bcc

import (
	"errors"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// isRestartLossReceipt reports whether an authenticated receipt, already
// bound to the stored Apply command, says that an ingress BCC recorded as
// applied and ownership-verified was lost to an Agent restart and that the
// Agent verifiably released its reservation afterwards.
func isRestartLossReceipt(op PublicIngressOperation, r ingressauthority.ReceiptV1) bool {
	applied := op.Receipt
	return applied != nil && applied.Phase == "applied" && applied.OwnershipVerified &&
		op.RetireCommand == nil && op.RetireReceipt == nil && op.Command != nil &&
		r.Action == agentjob.ActionPublicIngressApply && r.Phase == "failed" && !r.OwnershipVerified &&
		r.RollbackStatus == ingressauthority.RollbackReleasedAfterRestart && r.FailureCause != "" &&
		r.ListenerEvidence == (ingressauthority.ListenerEvidenceV1{}) && len(r.FirewallEvidence) == 0
}

// restageIngressAfterRestartLossLocked answers a restart-loss receipt with a
// fresh, HELD Apply for the same reviewed plan and acknowledged port at the
// next generation (ADR-003 §4.4). Nothing is adopted or re-bound by guess: the
// new command passes the production enqueue gate and the Agent's full Apply
// admission (port free, latent claims, policy pin, generation fence) exactly
// like the first one. The old command's lifetime is deliberately not required:
// a reboot can come days later, and the receipt is still bound to the exact
// stored command and the enrolled Agent credential; replay is impossible
// because restaging replaces that command. Fail-closed: any mismatch leaves
// the state unchanged.
func (s *Store) restageIngressAfterRestartLossLocked(op PublicIngressOperation, loss ingressauthority.ReceiptV1, now time.Time) error {
	if !isRestartLossReceipt(op, loss) || now.IsZero() {
		return errors.New("not a restart-loss receipt for an applied ingress")
	}
	old := *op.Command
	tunnelID := old.Binding.Spec.TunnelID
	t, ok := s.st.Tunnels[tunnelID]
	if !ok || t.Phase != TunnelActive || t.PlanHash != op.ReviewedPlanHash || t.ClientPort != op.Spec.PublicPort ||
		t.IRNode != op.Spec.IRNode || t.RouteListen != op.Spec.LocalTarget {
		return errors.New("restart-loss restage needs the same active reviewed tunnel")
	}
	if old.Action != agentjob.ActionPublicIngressApply || old.Binding.Spec.InstanceID != t.instanceForNode(t.IRNode) ||
		old.Binding.Spec.Generation != op.Proof.Generation || old.Binding.OperationID != op.OperationID || old.Binding.LeaseID != op.LeaseID {
		return errors.New("stored ingress command is inconsistent with its operation")
	}
	generation := op.Proof.Generation + 1
	if next := s.nextIngressGenerationLocked(op.Spec.IRNode, op.Spec.PublicPort); next > generation {
		generation = next
	}
	manifest := publicIngressManifestSpec(op.Spec, t.instanceForNode(t.IRNode), generation)
	preview, err := ingressmanifest.Preview(manifest)
	if err != nil {
		return err
	}
	proof := op.Proof
	proof.Generation, proof.ManifestDigest = generation, preview.ManifestSHA256
	if !proof.valid() {
		return errors.New("noncanonical restaged ingress identity")
	}
	newOperationID, err := newIngressIdentity("op-")
	if err != nil {
		return err
	}
	newLeaseID, err := newIngressIdentity("lease-")
	if err != nil {
		return err
	}
	newJobID, err := newIngressIdentity("apply-")
	if err != nil {
		return err
	}
	issued := now.UTC().Truncate(time.Second).Add(-time.Second)
	cmd := old
	cmd.JobID, cmd.IssuedAt, cmd.ExpiresAt = newJobID, issued, issued.Add(ingressHeldCommandWindow)
	cmd.Binding.OperationID, cmd.Binding.LeaseID = newOperationID, newLeaseID
	cmd.Binding.Spec, cmd.Binding.Proof = manifest, preview
	cmd.Binding.ListenEndpoint, cmd.Binding.LocalTarget = manifest.Config.Listen, manifest.Config.Target
	if err := cmd.Validate(now); err != nil {
		return err
	}
	if _, err := operationalIngressJob(cmd, now); err != nil {
		return err
	}
	for otherID, other := range s.st.PublicIngressOperations {
		if otherID == tunnelID {
			continue
		}
		if other.OperationID == newOperationID || other.LeaseID == newLeaseID ||
			(other.Command != nil && other.Command.JobID == newJobID) ||
			(other.RetireCommand != nil && other.RetireCommand.JobID == newJobID) {
			return errors.New("restaged ingress identity collision")
		}
	}
	if _, exists := s.st.Jobs[newJobID]; exists {
		return errors.New("restaged ingress JobID collides with queue")
	}
	before, err := cloneState(s.st)
	if err != nil {
		return err
	}
	op.OperationID, op.LeaseID, op.Proof = newOperationID, newLeaseID, proof
	op.Command, op.Receipt = &cmd, nil
	op.Dispatch = &IngressDispatchIntent{JobID: newJobID, OperationID: newOperationID, LeaseID: newLeaseID, Phase: ingressPhaseHeld, CreatedAt: now.UTC()}
	op.RestartRestages++
	s.st.PublicIngressOperations[tunnelID] = op
	if _, err := s.enqueueSecurityAuditLocked(AuditEntry{Timestamp: now, Actor: "bcc", Action: "public_ingress.apply.restage_after_restart", Target: tunnelID, Outcome: "success",
		Details: map[string]any{"superseded_job_id": old.JobID, "superseded_operation_id": old.Binding.OperationID, "superseded_lease_id": old.Binding.LeaseID,
			"superseded_generation": old.Binding.Spec.Generation, "job_id": newJobID, "operation_id": newOperationID, "lease_id": newLeaseID,
			"generation": generation, "reason": "agent restart rolled back an applied ingress and released its reservation"}}); err != nil {
		s.st = before
		return err
	}
	if err := s.saveLocked(); err != nil {
		s.st = before
		return err
	}
	return nil
}
