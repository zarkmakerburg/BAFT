package bcc

import (
	"errors"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"strconv"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
)

// CreatePublicIngressInspect queues one read-only, proof-bound observation.
// The expected proof comes only from BCC's persisted operation cursor.
func (s *Store) CreatePublicIngressInspect(tunnelID string, now time.Time, audit AuditEntry) (Job, error) {
	if tunnelID == "" || now.IsZero() {
		return Job{}, errors.New("inspect tunnel and timestamp are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	op, ok := s.st.PublicIngressOperations[tunnelID]
	reservation, reserved := s.st.PublicIngressReservations[tunnelID]
	if !ok || !reserved || op.Status != PublicIngressPending || op.Spec != reservation.Spec || op.Spec.TunnelID != tunnelID || !op.Proof.valid() {
		return Job{}, errors.New("inspect requires a matching pending operation and reservation")
	}
	node, ok := s.st.Nodes[op.Spec.IRNode]
	if !ok || node.Revoked || node.AgentTokenHash == "" || (node.Role != "worker" && node.Role != "master") {
		return Job{}, errors.New("inspect requires an active enrolled IR node")
	}
	if node.AgentSeen.IsZero() || now.Sub(node.AgentSeen) > 5*time.Minute || node.AgentSeen.After(now.Add(agentjob.ClockSkew)) {
		return Job{}, errors.New("inspect requires recent authenticated agent contact")
	}
	tunnel, ok := s.st.Tunnels[tunnelID]
	if !ok || tunnel.IRNode != op.Spec.IRNode || tunnel.RouteListen != op.Spec.LocalTarget || !tunnelKeepsPorts(tunnel) {
		return Job{}, errors.New("inspect operation no longer matches an active tunnel")
	}
	// A cursor and reservation may survive unrelated state edits or a restore.
	// Never sign fresh inspection work for a no-longer-canonical manifest.
	preview, err := ingressmanifest.Preview(publicIngressManifestSpec(op.Spec, tunnel.InstanceID, op.Proof.Generation))
	if err != nil || preview.ManifestSHA256 != op.Proof.ManifestDigest {
		return Job{}, errors.New("inspect operation proof differs from current tunnel configuration")
	}
	params := map[string]string{
		"tunnel_id": tunnelID, "public_port": strconv.Itoa(op.Spec.PublicPort), "local_target": op.Spec.LocalTarget,
		"generation": strconv.FormatUint(op.Proof.Generation, 10), "manifest_digest": op.Proof.ManifestDigest,
		"listener_id": op.Proof.ListenerID, "firewall_id": op.Proof.FirewallID,
	}
	if err := (agentjob.Job{SchemaVersion: agentjob.SchemaVersion, JobID: "inspect-validation", NodeID: op.Spec.IRNode, Action: agentjob.ActionPublicIngressInspect, Params: params, IssuedAt: now, ExpiresAt: now.Add(time.Minute)}).Validate(); err != nil {
		return Job{}, err
	}
	for _, old := range s.st.Jobs {
		if old.Type == agentjob.ActionPublicIngressInspect && old.NodeID == op.Spec.IRNode && old.Params["tunnel_id"] == tunnelID && (old.Status == "queued" || old.Status == "dispatched") && now.Before(publicIngressPreflightDeadline(old)) {
			return Job{}, errors.New("an ingress inspection is already pending")
		}
	}
	before, err := cloneState(s.st)
	if err != nil {
		return Job{}, err
	}
	for id, old := range s.st.Jobs {
		if old.Type != agentjob.ActionPublicIngressInspect || old.NodeID != op.Spec.IRNode || old.Params["tunnel_id"] != tunnelID || (old.Status != "queued" && old.Status != "dispatched") {
			continue
		}
		old.Status, old.Message, old.UpdatedAt = "failed", "read-only ingress inspection expired before replacement", now.UTC()
		s.st.Jobs[id] = old
		if _, err := s.enqueueSecurityAuditLocked(AuditEntry{Timestamp: now.UTC(), Actor: "bcc", Action: "public_ingress.inspect.expire", Target: tunnelID, Outcome: "expired", Details: map[string]any{"job_id": id, "ir_node": old.NodeID, "public_port": old.Params["public_port"]}}); err != nil {
			s.st = before
			return Job{}, err
		}
	}
	job := s.newJobLocked(Job{Type: agentjob.ActionPublicIngressInspect, NodeID: op.Spec.IRNode, Params: params})
	job.CreatedAt, job.UpdatedAt = now.UTC(), now.UTC()
	s.st.Jobs[job.ID] = job
	audit.Action, audit.Target, audit.Outcome = "public_ingress.inspect", tunnelID, "success"
	audit.Details = mergeAuditDetails(audit.Details, map[string]any{"job_id": job.ID, "ir_node": job.NodeID, "generation": op.Proof.Generation, "manifest_digest": op.Proof.ManifestDigest, "read_only": true})
	if _, err := s.enqueueSecurityAuditLocked(audit); err != nil {
		s.st = before
		return Job{}, err
	}
	if err := s.saveLocked(); err != nil {
		s.st = before
		return Job{}, err
	}
	return job, nil
}

// validateCurrentPublicIngressInspectIntent fences late ACKs against a changed
// durable operation. Receipt echo alone cannot establish current authority.
func validateCurrentPublicIngressInspectIntent(st state, j Job) error {
	op, ok := st.PublicIngressOperations[j.Params["tunnel_id"]]
	if !ok || op.Status != PublicIngressPending || op.Spec.IRNode != j.NodeID || op.Spec.TunnelID != j.Params["tunnel_id"] || strconv.Itoa(op.Spec.PublicPort) != j.Params["public_port"] || op.Spec.LocalTarget != j.Params["local_target"] || strconv.FormatUint(op.Proof.Generation, 10) != j.Params["generation"] || op.Proof.ManifestDigest != j.Params["manifest_digest"] || op.Proof.ListenerID != j.Params["listener_id"] || op.Proof.FirewallID != j.Params["firewall_id"] {
		return errors.New("ingress inspection ACK no longer matches durable operation")
	}
	reservation, ok := st.PublicIngressReservations[op.Spec.TunnelID]
	if !ok || reservation.Spec != op.Spec {
		return errors.New("ingress inspection ACK no longer matches reservation")
	}
	tunnel, ok := st.Tunnels[op.Spec.TunnelID]
	if !ok || tunnel.IRNode != op.Spec.IRNode || tunnel.RouteListen != op.Spec.LocalTarget || !tunnelKeepsPorts(tunnel) {
		return errors.New("ingress inspection ACK no longer matches tunnel")
	}
	preview, err := ingressmanifest.Preview(publicIngressManifestSpec(op.Spec, tunnel.InstanceID, op.Proof.Generation))
	if err != nil || preview.ManifestSHA256 != op.Proof.ManifestDigest {
		return errors.New("ingress inspection ACK uses stale canonical manifest")
	}
	return nil
}
