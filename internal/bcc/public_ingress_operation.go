package bcc

import (
	"errors"
	"net"
	"strconv"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

func publicIngressManifestSpec(spec PublicIngressSpec, instance string, generation uint64) ingressmanifest.Spec {
	return ingressmanifest.Spec{TunnelID: spec.TunnelID, InstanceID: instance, Generation: generation, Config: ingressmanifest.Config{
		Listen: net.JoinHostPort("0.0.0.0", strconv.Itoa(spec.PublicPort)), Target: spec.LocalTarget,
		MaxClients: 128, DialTimeoutMS: 3000,
	}}
}

// PublicIngressOperation is BCC's durable expected proof for one reserved
// ingress. PENDING does not authorize or prove any agent-side host change.
type PublicIngressOperation struct {
	ReviewedPlanHash string                     `json:"reviewed_plan_hash,omitempty"` // Exact BCC-reviewed tunnel plan; legacy desired-only intents omit it.
	Spec             PublicIngressSpec          `json:"spec"`
	Proof            PublicIngressExpectedProof `json:"proof"`
	Status           string                     `json:"status"`
	CreatedAt        time.Time                  `json:"created_at"`
}

// RecordPublicIngressOperationIntent fences a reservation to one generation
// and resource identity before a future signed agent operation is issued.
// Replacing an existing intent requires a separate recovery protocol.
func (s *Store) RecordPublicIngressOperationIntent(spec PublicIngressSpec, proof PublicIngressExpectedProof, now time.Time, audit AuditEntry) (PublicIngressOperation, error) {
	canonical, err := PlanPublicIngress(spec.IRNode, spec.TunnelID, spec.PublicPort, spec.LocalTarget)
	if err != nil || canonical != spec || !proof.valid() || now.IsZero() {
		return PublicIngressOperation{}, errors.New("invalid public ingress operation intent")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	reservation, ok := s.st.PublicIngressReservations[spec.TunnelID]
	if !ok || reservation.Spec != spec {
		return PublicIngressOperation{}, errors.New("public ingress operation requires the exact reservation")
	}
	tunnel, ok := s.st.Tunnels[spec.TunnelID]
	if !ok || tunnel.IRNode != spec.IRNode || tunnel.RouteListen != spec.LocalTarget || !tunnelKeepsPorts(tunnel) {
		return PublicIngressOperation{}, errors.New("public ingress operation requires the current active tunnel")
	}
	manifestProof, err := ingressmanifest.Preview(publicIngressManifestSpec(spec, tunnel.InstanceID, proof.Generation))
	if err != nil || manifestProof.ManifestSHA256 != proof.ManifestDigest {
		return PublicIngressOperation{}, errors.New("public ingress manifest digest differs from canonical reserved configuration")
	}
	if existing, ok := s.st.PublicIngressOperations[spec.TunnelID]; ok {
		if existing.Spec == spec && existing.Proof == proof && existing.Status == PublicIngressPending {
			return existing, nil
		}
		return PublicIngressOperation{}, errors.New("public ingress operation already fenced to another proof or state")
	}
	before, err := cloneState(s.st)
	if err != nil {
		return PublicIngressOperation{}, err
	}
	if s.st.PublicIngressOperations == nil {
		s.st.PublicIngressOperations = map[string]PublicIngressOperation{}
	}
	op := PublicIngressOperation{Spec: spec, Proof: proof, Status: PublicIngressPending, CreatedAt: now.UTC()}
	s.st.PublicIngressOperations[spec.TunnelID] = op
	audit.Action, audit.Target, audit.Outcome = "public_ingress.operation.intent", spec.TunnelID, "success"
	audit.Details = mergeAuditDetails(audit.Details, map[string]any{"generation": proof.Generation, "manifest_digest": proof.ManifestDigest, "listener_id": proof.ListenerID, "firewall_id": proof.FirewallID, "ir_node": spec.IRNode, "public_port": spec.PublicPort})
	if _, err := s.enqueueSecurityAuditLocked(audit); err != nil {
		s.st = before
		return PublicIngressOperation{}, err
	}
	if err := s.saveLocked(); err != nil {
		s.st = before
		return PublicIngressOperation{}, err
	}
	return op, nil
}

// Restore preserves current operation fences and refuses a backup that would
// reinterpret a tunnel under a different generation or resource identity.
func mergePublicIngressOperations(restored *state, current state) error {
	merged := make(map[string]PublicIngressOperation, len(restored.PublicIngressOperations)+len(current.PublicIngressOperations))
	for id, op := range restored.PublicIngressOperations {
		merged[id] = op
	}
	for id, op := range current.PublicIngressOperations {
		if old, ok := merged[id]; ok && (old.Spec != op.Spec || old.Proof != op.Proof || old.ReviewedPlanHash != op.ReviewedPlanHash) {
			return errors.New("backup changes a current public ingress operation proof")
		}
		merged[id] = op
	}
	for id, op := range merged {
		reservation, ok := restored.PublicIngressReservations[id]
		if !ok || id != op.Spec.TunnelID || reservation.Spec != op.Spec || !op.Proof.valid() || op.Status != PublicIngressPending || op.CreatedAt.IsZero() {
			return errors.New("invalid restored public ingress operation")
		}
		// The restored operation proof must still match the canonical manifest
		// for the restored tunnel; a matching reservation alone is insufficient.
		tunnel, exists := restored.Tunnels[id]
		if !exists || tunnel.IRNode != op.Spec.IRNode || tunnel.RouteListen != op.Spec.LocalTarget || !tunnelKeepsPorts(tunnel) {
			return errors.New("restored public ingress operation has no matching active tunnel")
		}
		if op.ReviewedPlanHash != "" && (tunnel.PlanHash != op.ReviewedPlanHash || tunnel.ClientPort != op.Spec.PublicPort || tunnel.Phase != TunnelActive) {
			return errors.New("restored reviewed ingress operation is no longer bound to active tunnel plan and client port")
		}
		preview, err := ingressmanifest.Preview(publicIngressManifestSpec(op.Spec, tunnel.InstanceID, op.Proof.Generation))
		if err != nil || preview.ManifestSHA256 != op.Proof.ManifestDigest {
			return errors.New("restored public ingress operation has noncanonical proof")
		}
	}
	restored.PublicIngressOperations = merged
	return nil
}
