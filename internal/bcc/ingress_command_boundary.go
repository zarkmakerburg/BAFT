package bcc

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"reflect"
	"time"
)

// Staged commands remain inert until independently authorized.
func newIngressIdentity(prefix string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(b[:]), nil
}

func (s *Store) StageReviewedIngressCommand(tunnelID string, command ingressauthority.CommandV1, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stageReviewedIngressCommandLocked(tunnelID, command, now, true)
}

func (s *Store) stageReviewedIngressCommandLocked(tunnelID string, command ingressauthority.CommandV1, now time.Time, persist bool) error {
	if err := command.Validate(now); err != nil {
		return err
	}
	if command.Action != "public_ingress_apply" {
		return errors.New("retire requires separate authorization")
	}
	// Persist exactly what the normal signed Agent job can carry: signing
	// truncates to whole seconds, caps the validity window and admits only the
	// bounded v1 parameter schema. Refuse anything else BEFORE durable staging
	// so a lossy or unsignable command can never become a reviewed identity.
	if _, err := operationalIngressJob(command, now); err != nil {
		return err
	}
	op, ok := s.st.PublicIngressOperations[tunnelID]
	if !ok || op.Status != PublicIngressPending || op.ReviewedPlanHash == "" || op.OperationID == "" || op.LeaseID == "" {
		return errors.New("missing durable reviewed operation")
	}
	tun, ok := s.st.Tunnels[tunnelID]
	if !ok || tun.Phase != TunnelActive || tun.PlanHash != op.ReviewedPlanHash || tun.ClientPort != op.Spec.PublicPort || tun.IRNode != op.Spec.IRNode || tun.RouteListen != op.Spec.LocalTarget {
		return errors.New("reviewed tunnel changed")
	}
	b := command.Binding
	if b.NodeID != op.Spec.IRNode || b.OperationID != op.OperationID || b.LeaseID != op.LeaseID ||
		b.PlanSHA256 != op.ReviewedPlanHash || int(b.Port) != op.Spec.PublicPort ||
		int(b.ConfigPort) != op.Spec.PublicPort || b.Spec.TunnelID != tunnelID ||
		b.Spec.InstanceID != tun.InstanceID || b.Spec.Generation != op.Proof.Generation ||
		b.ListenerID != op.Proof.ListenerID || b.FirewallID != op.Proof.FirewallID ||
		b.Proof.ManifestSHA256 != op.Proof.ManifestDigest {
		return errors.New("binding differs from persisted review")
	}
	expectedSpec := publicIngressManifestSpec(op.Spec, tun.InstanceID, op.Proof.Generation)
	expectedProof, err := ingressmanifest.Preview(expectedSpec)
	if err != nil || b.Spec != expectedSpec || b.Proof != expectedProof {
		return errors.New("noncanonical staged manifest")
	}
	if op.Command != nil {
		if reflect.DeepEqual(*op.Command, command) {
			return nil
		}
		return errors.New("staged command cannot change identity")
	}
	// A signed job ID is an immutable replay identity shared by all staged
	// operations. Refuse collisions before persisting a state OpenStore
	// would reject, or an Agent replay ledger would conflate.
	for otherID, other := range s.st.PublicIngressOperations {
		if otherID != tunnelID && other.Command != nil && other.Command.JobID == command.JobID {
			return errors.New("signed ingress job ID already belongs to another operation")
		}
	}
	before, err := cloneState(s.st)
	if err != nil {
		return err
	}
	op.Command = &command
	s.st.PublicIngressOperations[tunnelID] = op
	if persist {
		if err := s.saveLocked(); err != nil {
			s.st = before
			return err
		}
	}
	return nil
}

// A matching Agent POST remains unverified observation until a real held
// authority and independent executor proof can reconcile it.
func (s *Store) RecordIngressAgentReceipt(receipt ingressauthority.ReceiptV1, token string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := receipt.Binding
	if _, ok := s.authorizedHashLocked(b.NodeID, token, now); !ok {
		return ErrAgentAuthentication
	}
	if receipt.SchemaVersion != 1 || receipt.JobID == "" || receipt.Action == "" {
		return errors.New("invalid receipt schema")
	}
	op, ok := s.st.PublicIngressOperations[b.Spec.TunnelID]
	if !ok || op.Command == nil || op.Status != PublicIngressPending {
		return errors.New("missing staged command or duplicate receipt")
	}
	if op.RetireCommand != nil && receipt.JobID == op.RetireCommand.JobID {
		return s.recordRetireReceiptLocked(op, receipt, now)
	}
	cmd := *op.Command
	if cmd.JobID != receipt.JobID || cmd.Action != receipt.Action || cmd.Binding != b ||
		b.PlanSHA256 != op.ReviewedPlanHash || b.OperationID != op.OperationID || b.LeaseID != op.LeaseID ||
		b.NodeID != op.Spec.IRNode || uint16(op.Spec.PublicPort) != b.Port ||
		b.Spec.Generation != op.Proof.Generation || b.Proof.ManifestSHA256 != op.Proof.ManifestDigest {
		return errors.New("receipt not bound to stored command")
	}
	if err := b.MatchReviewed(cmd.Binding); err != nil {
		return err
	}
	if op.Receipt != nil {
		if reflect.DeepEqual(*op.Receipt, receipt) {
			return nil
		}
		return errors.New("conflicting ingress receipt replay")
	}
	if now.IsZero() || now.Before(cmd.IssuedAt) || !now.Before(cmd.ExpiresAt) {
		return errors.New("receipt outside command lifetime")
	}
	before, err := cloneState(s.st)
	if err != nil {
		return err
	}
	receipt.FirewallEvidence = append([]ingressauthority.FirewallEvidenceV1(nil), receipt.FirewallEvidence...)
	op.Receipt = &receipt
	// Even Phase=applied does not change the PENDING state.
	s.st.PublicIngressOperations[b.Spec.TunnelID] = op
	if err := s.saveLocked(); err != nil {
		s.st = before
		return err
	}
	return nil
}

// recordRetireReceiptLocked stores an exact Retire receipt as inert evidence.
// It never marks the ingress REMOVED; absence must be independently observed.
func (s *Store) recordRetireReceiptLocked(op PublicIngressOperation, receipt ingressauthority.ReceiptV1, now time.Time) error {
	rc := *op.RetireCommand
	if op.Receipt == nil || rc.Action != receipt.Action || rc.Binding != receipt.Binding || receipt.SchemaVersion != 1 {
		return errors.New("retire receipt not bound to stored retire command")
	}
	if op.RetireReceipt != nil {
		if reflect.DeepEqual(*op.RetireReceipt, receipt) {
			return nil
		}
		return errors.New("conflicting ingress retire receipt replay")
	}
	if now.IsZero() || now.Before(rc.IssuedAt) || !now.Before(rc.ExpiresAt) {
		return errors.New("retire receipt outside command lifetime")
	}
	before, err := cloneState(s.st)
	if err != nil {
		return err
	}
	receipt.FirewallEvidence = append([]ingressauthority.FirewallEvidenceV1(nil), receipt.FirewallEvidence...)
	op.RetireReceipt = &receipt
	s.st.PublicIngressOperations[rc.Binding.Spec.TunnelID] = op
	if err := s.saveLocked(); err != nil {
		s.st = before
		return err
	}
	return nil
}
