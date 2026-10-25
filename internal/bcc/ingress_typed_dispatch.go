package bcc

import (
	"errors"
	"reflect"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
)

func isOperationalIngressJob(j Job) bool {
	return j.Type == agentjob.ActionPublicIngressApply || j.Type == agentjob.ActionPublicIngressRetire
}

// This is data conversion, never authority to mutate a node. Public dispatch
// remains gated until the normal Agent operational lifecycle is accepted.
func operationalIngressJobCommand(j Job, now time.Time) (agentjob.Job, error) {
	if !isOperationalIngressJob(j) || j.IngressCommand == nil {
		return agentjob.Job{}, errors.New("operational job lacks its reviewed command")
	}
	c := *j.IngressCommand
	if err := c.Validate(now); err != nil {
		return agentjob.Job{}, err
	}
	signed, err := c.AgentJob()
	if err != nil {
		return agentjob.Job{}, err
	}
	if j.ID != c.JobID || j.Type != c.Action || j.NodeID != c.Binding.NodeID || !reflect.DeepEqual(j.Params, signed.Params) || !j.CreatedAt.Equal(c.IssuedAt) {
		return agentjob.Job{}, errors.New("operational queue record differs from reviewed command")
	}
	return signed, nil
}

func cloneIngressJob(j Job) Job {
	if j.IngressCommand != nil {
		c := *j.IngressCommand
		j.IngressCommand = &c
	}
	if j.Params != nil {
		params := make(map[string]string, len(j.Params))
		for k, v := range j.Params {
			params[k] = v
		}
		j.Params = params
	}
	return j
}

// enqueueReviewedIngress is the public dispatch boundary for the exact reviewed
// signed command. The Agent independently establishes its held private lease
// and revalidates the installed LAB pin before effects; queueing alone proves
// neither Apply nor readiness, and unsupported hosts return a BLOCKED receipt.
func (s *Store) enqueueReviewedIngress(tunnelID string, now time.Time, audit AuditEntry) (Job, error) {
	return s.enqueueReviewedIngressAction(tunnelID, now, audit, false)
}

func (s *Store) enqueueReviewedIngressAction(tunnelID string, now time.Time, audit AuditEntry, retiring bool) (Job, error) {
	if now.IsZero() {
		return Job{}, errors.New("dispatch needs current time")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	op, ok := s.st.PublicIngressOperations[tunnelID]
	command, dispatch, receipt := ingressQueuedParts(op, retiring)
	if !ok || op.Status != PublicIngressPending || command == nil || dispatch == nil || receipt != nil {
		return Job{}, ErrIngressAuthorityUnavailable
	}
	if err := validatePersistedIngressOperations(s.st); err != nil {
		return Job{}, err
	}
	c := *command
	if dispatch.Phase == "queued" {
		j, ok := s.st.Jobs[c.JobID]
		if !ok {
			return Job{}, errors.New("queued ingress record absent")
		}
		return cloneIngressJob(j), nil
	}
	if dispatch.Phase != "held" {
		return Job{}, errors.New("ingress command is not held")
	}
	if err := c.Validate(now); err != nil {
		return Job{}, err
	}
	node, ok := s.st.Nodes[c.Binding.NodeID]
	if !ok || node.Revoked || node.AgentTokenHash == "" || (node.Role != "worker" && node.Role != "master") || node.AgentSeen.IsZero() || now.Sub(node.AgentSeen) > 5*time.Minute || node.AgentSeen.After(now.Add(agentjob.ClockSkew)) {
		return Job{}, errors.New("dispatch needs recent enrolled IR contact")
	}
	wire, err := c.AgentJob()
	if err != nil {
		return Job{}, err
	}
	if _, exists := s.st.Jobs[c.JobID]; exists {
		return Job{}, errors.New("ingress job identity already queued")
	}
	before, err := cloneState(s.st)
	if err != nil {
		return Job{}, err
	}
	j := Job{ID: c.JobID, Type: c.Action, NodeID: c.Binding.NodeID, IngressCommand: &c, Params: wire.Params, Status: "queued", CreatedAt: c.IssuedAt, UpdatedAt: now.UTC()}
	d := *dispatch
	d.Phase = "queued"
	if retiring {
		retirement := *op.Retirement
		retirement.Dispatch = d
		op.Retirement = &retirement
	} else {
		op.Dispatch = &d
	}
	s.st.PublicIngressOperations[tunnelID] = op
	s.st.Jobs[j.ID] = j
	audit.Timestamp = now.UTC()
	audit.Action = "public_ingress.dispatch"
	audit.Target = tunnelID
	audit.Outcome = "queued"
	audit.Details = mergeAuditDetails(audit.Details, map[string]any{"job_id": j.ID, "operation_id": op.OperationID, "lease_id": op.LeaseID, "plan_hash": op.ReviewedPlanHash, "public_port": op.Spec.PublicPort})
	if _, err := s.enqueueSecurityAuditLocked(audit); err != nil {
		s.st = before
		return Job{}, err
	}
	if err := s.saveLocked(); err != nil {
		s.st = before
		return Job{}, err
	}
	return cloneIngressJob(j), nil
}
