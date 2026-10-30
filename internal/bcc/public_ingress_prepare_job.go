package bcc

import (
	"errors"
	"strconv"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
)

// CreatePublicIngressPrepare queues an isolated private-manifest write for the
// current fenced operation. It does not authorize a host listener or firewall.
func (s *Store) CreatePublicIngressPrepare(tunnelID string, now time.Time, audit AuditEntry) (Job, error) {
	if tunnelID == "" || now.IsZero() {
		return Job{}, errors.New("prepare tunnel and timestamp are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	op, ok := s.st.PublicIngressOperations[tunnelID]
	reservation, reserved := s.st.PublicIngressReservations[tunnelID]
	if !ok || !reserved || op.Status != PublicIngressPending || op.Spec != reservation.Spec || op.Spec.TunnelID != tunnelID || !op.Proof.valid() || op.Proof.Generation == 0 {
		return Job{}, errors.New("prepare requires a matching initial operation and reservation")
	}
	tunnel, ok := s.st.Tunnels[tunnelID]
	if !ok || tunnel.IRNode != op.Spec.IRNode || tunnel.RouteListen != op.Spec.LocalTarget || !tunnelKeepsPorts(tunnel) {
		return Job{}, errors.New("prepare operation no longer matches an active tunnel")
	}
	node, ok := s.st.Nodes[op.Spec.IRNode]
	if !ok || node.Revoked || node.AgentTokenHash == "" || (node.Role != "worker" && node.Role != "master") || node.AgentSeen.IsZero() || now.Sub(node.AgentSeen) > 5*time.Minute || node.AgentSeen.After(now.Add(agentjob.ClockSkew)) {
		return Job{}, errors.New("prepare requires recent authenticated IR agent contact")
	}
	params := map[string]string{
		"tunnel_id": tunnelID, "public_port": strconv.Itoa(op.Spec.PublicPort), "local_target": op.Spec.LocalTarget,
		"generation": "1", "manifest_digest": op.Proof.ManifestDigest, "listener_id": op.Proof.ListenerID, "firewall_id": op.Proof.FirewallID,
	}
	if instance := tunnel.instanceForNode(tunnel.IRNode); instance != "" {
		params["instance_id"] = instance
	}
	if err := (agentjob.Job{SchemaVersion: agentjob.SchemaVersion, JobID: "prepare-validation", NodeID: op.Spec.IRNode, Action: agentjob.ActionPublicIngressPrepare, Params: params, IssuedAt: now, ExpiresAt: now.Add(time.Minute)}).Validate(); err != nil {
		return Job{}, err
	}
	for _, old := range s.st.Jobs {
		if old.Type == agentjob.ActionPublicIngressPrepare && old.NodeID == op.Spec.IRNode && old.Params["tunnel_id"] == tunnelID {
			return Job{}, errors.New("ingress prepare already issued; inspect or recover its outcome")
		}
	}
	before, err := cloneState(s.st)
	if err != nil {
		return Job{}, err
	}
	job := s.newJobLocked(Job{Type: agentjob.ActionPublicIngressPrepare, NodeID: op.Spec.IRNode, Params: params})
	job.CreatedAt, job.UpdatedAt = now.UTC(), now.UTC()
	s.st.Jobs[job.ID] = job
	audit.Action, audit.Target, audit.Outcome = "public_ingress.prepare", tunnelID, "success"
	audit.Details = mergeAuditDetails(audit.Details, map[string]any{"job_id": job.ID, "ir_node": job.NodeID, "generation": op.Proof.Generation, "manifest_digest": op.Proof.ManifestDigest, "prepared_only": true})
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
