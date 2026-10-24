package bcc

import (
	"errors"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// RecordReviewedPublicIngressOperation atomically records BCC's desired-only
// reservation and pending operation. This does NOT enqueue an Agent Apply,
// claim a host listener, prove foreign service ownership, or mint a lease.
func (s *Store) RecordReviewedPublicIngressOperation(spec PublicIngressSpec, planHash string, proof PublicIngressExpectedProof, now time.Time, audit AuditEntry) (PublicIngressOperation, error) {
	canonical, err := PlanPublicIngress(spec.IRNode, spec.TunnelID, spec.PublicPort, spec.LocalTarget)
	if err != nil || canonical != spec || now.IsZero() || !proof.valid() || proof.Generation != 1 {
		return PublicIngressOperation{}, errors.New("invalid reviewed ingress operation identity or proof")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tunnel, ok := s.st.Tunnels[spec.TunnelID]
	if !ok || tunnel.Phase != TunnelActive || tunnel.PlanHash == "" || tunnel.PlanHash != planHash || tunnel.ClientPort != spec.PublicPort || tunnel.IRNode != spec.IRNode || tunnel.RouteListen != spec.LocalTarget {
		return PublicIngressOperation{}, errors.New("reviewed tunnel and canonical client port must match active persisted plan")
	}
	node, ok := s.st.Nodes[spec.IRNode]
	if !ok || node.Revoked || node.AgentTokenHash == "" || (node.Role != "worker" && node.Role != "master") {
		return PublicIngressOperation{}, errors.New("IR node is not enrolled or authorized")
	}
	manifest, err := ingressmanifest.Preview(publicIngressManifestSpec(spec, tunnel.InstanceID, proof.Generation))
	if err != nil || manifest.ManifestSHA256 != proof.ManifestDigest {
		return PublicIngressOperation{}, errors.New("noncanonical ingress operation manifest proof")
	}
	if existing, exists := s.st.PublicIngressOperations[spec.TunnelID]; exists {
		if existing.Spec == spec && existing.Proof == proof && existing.ReviewedPlanHash == planHash && existing.Status == PublicIngressPending {
			reservation, ok := s.st.PublicIngressReservations[spec.TunnelID]
			if !ok || reservation.Spec != spec {
				return PublicIngressOperation{}, errors.New("inconsistent operation and reservation")
			}
			return existing, nil
		}
		return PublicIngressOperation{}, errors.New("ingress operation already fenced")
	}
	if existing, exists := s.st.PublicIngressReservations[spec.TunnelID]; exists && existing.Spec != spec {
		return PublicIngressOperation{}, errors.New("ingress reservation already belongs to another tuple")
	}
	specs, err := s.publicIngressReservationSpecsLocked()
	if err != nil {
		return PublicIngressOperation{}, err
	}
	if err := ValidatePublicIngressReservation(spec, specs); err != nil {
		return PublicIngressOperation{}, err
	}
	if err := s.validateIngressReservationResourcesLocked(spec); err != nil {
		return PublicIngressOperation{}, err
	}
	before, err := cloneState(s.st)
	if err != nil {
		return PublicIngressOperation{}, err
	}
	if s.st.PublicIngressReservations == nil {
		s.st.PublicIngressReservations = map[string]PublicIngressReservation{}
	}
	if s.st.PublicIngressOperations == nil {
		s.st.PublicIngressOperations = map[string]PublicIngressOperation{}
	}
	if _, exists := s.st.PublicIngressReservations[spec.TunnelID]; !exists {
		s.st.PublicIngressReservations[spec.TunnelID] = PublicIngressReservation{Spec: spec, CreatedAt: now.UTC()}
	}
	opID, err := newIngressIdentity("op-")
	if err != nil {
		s.st = before
		return PublicIngressOperation{}, err
	}
	leaseID, err := newIngressIdentity("lease-")
	if err != nil {
		s.st = before
		return PublicIngressOperation{}, err
	}
	op := PublicIngressOperation{Spec: spec, Proof: proof, ReviewedPlanHash: planHash, OperationID: opID, LeaseID: leaseID, Status: PublicIngressPending, CreatedAt: now.UTC()}
	s.st.PublicIngressOperations[spec.TunnelID] = op
	audit.Action, audit.Target, audit.Outcome = "public_ingress.operation.reviewed_intent", spec.TunnelID, "success"
	audit.Details = mergeAuditDetails(audit.Details, map[string]any{"ir_node": spec.IRNode, "public_port": spec.PublicPort, "client_port": spec.PublicPort, "plan_hash": planHash, "generation": proof.Generation, "manifest_digest": proof.ManifestDigest, "desired_only": true})
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
