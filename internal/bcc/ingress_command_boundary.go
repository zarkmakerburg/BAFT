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
	if err := command.Validate(now); err != nil {
		return err
	}
	if command.Action != "public_ingress_apply" {
		return errors.New("retire requires separate authorization")
	}
	// Persist exactly what the normal signed Agent can consume. Signing rounds
	// timestamps to seconds and admits only its bounded v1 parameter schema.
	if _, err := command.AgentJob(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
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
	if err := s.saveLocked(); err != nil {
		s.st = before
		return err
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
	retiring := receipt.Action == "public_ingress_retire"
	command, dispatch, oldReceipt := ingressQueuedParts(op, retiring)
	if command == nil {
		return errors.New("missing expected operational command")
	}
	cmd := *command
	if cmd.JobID != receipt.JobID || cmd.Action != receipt.Action || cmd.Binding != b ||
		b.PlanSHA256 != op.ReviewedPlanHash || b.OperationID != op.OperationID || b.LeaseID != op.LeaseID ||
		b.NodeID != op.Spec.IRNode || uint16(op.Spec.PublicPort) != b.Port ||
		b.Spec.Generation != op.Proof.Generation || b.Proof.ManifestSHA256 != op.Proof.ManifestDigest {
		return errors.New("receipt not bound to stored command")
	}
	if err := b.MatchReviewed(cmd.Binding); err != nil {
		return err
	}
	if oldReceipt != nil {
		if reflect.DeepEqual(*oldReceipt, receipt) {
			return nil
		}
		return errors.New("conflicting ingress receipt replay")
	}
	if now.IsZero() || now.Before(cmd.IssuedAt) || !now.Before(cmd.ExpiresAt) {
		return errors.New("receipt outside command lifetime")
	}
	// Only a command dispatched through the typed operational queue may
	// accept coherent positive execution evidence. This remains a node's
	// authenticated observation, not a local mutation permit or Wizard Ready.
	if dispatch != nil && dispatch.Phase == "queued" {
		job, exists := s.st.Jobs[cmd.JobID]
		if !exists || job.Status != "dispatched" || job.IngressCommand == nil || *job.IngressCommand != cmd {
			return errors.New("operational receipt requires exact dispatched job")
		}
		if err := receipt.ValidateForCommand(cmd); err != nil {
			return err
		}
	}
	before, err := cloneState(s.st)
	if err != nil {
		return err
	}
	receipt.FirewallEvidence = append([]ingressauthority.FirewallEvidenceV1(nil), receipt.FirewallEvidence...)
	if retiring {
		if receipt.Phase == "retired" && (op.Receipt == nil || !reflect.DeepEqual(receipt.FirewallEvidence, op.Receipt.FirewallEvidence)) {
			return errors.New("retirement changed original firewall identity")
		}
		retired := *op.Retirement
		retired.Receipt = &receipt
		op.Retirement = &retired
	} else {
		op.Receipt = &receipt
	}
	// Even Phase=applied does not change the PENDING state.
	s.st.PublicIngressOperations[b.Spec.TunnelID] = op
	if err := s.saveLocked(); err != nil {
		s.st = before
		return err
	}
	return nil
}
