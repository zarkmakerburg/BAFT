package bcc

import (
	"errors"
	"strconv"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// CreatePublicIngressCancel orders a signed local tombstone for a previously
// issued prepare. It cannot remove a listener or firewall resource.
func (s *Store) CreatePublicIngressCancel(tunnelID string, now time.Time, audit AuditEntry) (Job, error) {
	if tunnelID == "" || now.IsZero() {
		return Job{}, errors.New("cancel tunnel and timestamp are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	op, ok := s.st.PublicIngressOperations[tunnelID]
	reservation, reserved := s.st.PublicIngressReservations[tunnelID]
	if !ok || !reserved || op.Status != PublicIngressPending || op.Spec != reservation.Spec || op.Spec.TunnelID != tunnelID || !op.Proof.valid() || op.Proof.Generation != 1 {
		return Job{}, errors.New("cancel requires a matching initial operation and reservation")
	}
	tunnel, ok := s.st.Tunnels[tunnelID]
	if !ok || tunnel.IRNode != op.Spec.IRNode || tunnel.RouteListen != op.Spec.LocalTarget || !tunnelKeepsPorts(tunnel) {
		return Job{}, errors.New("cancel operation no longer matches an active tunnel")
	}
	preview, err := ingressmanifest.Preview(publicIngressManifestSpec(op.Spec, tunnel.InstanceID, 1))
	if err != nil || preview.ManifestSHA256 != op.Proof.ManifestDigest {
		return Job{}, errors.New("cancel operation has a stale canonical manifest")
	}
	node, ok := s.st.Nodes[op.Spec.IRNode]
	if !ok || node.Revoked || node.AgentTokenHash == "" || (node.Role != "worker" && node.Role != "master") || node.AgentSeen.IsZero() || now.Sub(node.AgentSeen) > 5*time.Minute || node.AgentSeen.After(now.Add(agentjob.ClockSkew)) {
		return Job{}, errors.New("cancel requires recent authenticated IR agent contact")
	}
	issuedPrepare := false
	for _, old := range s.st.Jobs {
		if old.NodeID != op.Spec.IRNode || old.Params["tunnel_id"] != tunnelID {
			continue
		}
		if old.Type == agentjob.ActionPublicIngressPrepare {
			issuedPrepare = true
		}
		if old.Type == agentjob.ActionPublicIngressCancel {
			if old.Status == "succeeded" || ((old.Status == "queued" || old.Status == "dispatched") && now.Before(publicIngressPreflightDeadline(old))) {
				return Job{}, errors.New("ingress cancellation already issued")
			}
		}
	}
	if !issuedPrepare {
		return Job{}, errors.New("cancel requires an issued ingress prepare")
	}
	params := map[string]string{
		"tunnel_id": tunnelID, "public_port": strconv.Itoa(op.Spec.PublicPort), "local_target": op.Spec.LocalTarget,
		"generation": "1", "manifest_digest": op.Proof.ManifestDigest, "listener_id": op.Proof.ListenerID, "firewall_id": op.Proof.FirewallID,
	}
	if tunnel.InstanceID != "" {
		params["instance_id"] = tunnel.InstanceID
	}
	if err := (agentjob.Job{SchemaVersion: agentjob.SchemaVersion, JobID: "cancel-validation", NodeID: op.Spec.IRNode, Action: agentjob.ActionPublicIngressCancel, Params: params, IssuedAt: now, ExpiresAt: now.Add(time.Minute)}).Validate(); err != nil {
		return Job{}, err
	}
	before, err := cloneState(s.st)
	if err != nil {
		return Job{}, err
	}
	job := s.newJobLocked(Job{Type: agentjob.ActionPublicIngressCancel, NodeID: op.Spec.IRNode, Params: params})
	job.CreatedAt, job.UpdatedAt = now.UTC(), now.UTC()
	s.st.Jobs[job.ID] = job
	audit.Action, audit.Target, audit.Outcome = "public_ingress.cancel", tunnelID, "success"
	audit.Details = mergeAuditDetails(audit.Details, map[string]any{"job_id": job.ID, "ir_node": job.NodeID, "generation": op.Proof.Generation, "manifest_digest": op.Proof.ManifestDigest, "manifest_only": true})
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
