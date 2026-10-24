package bcc

import (
	"errors"
	"reflect"
	"strconv"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
)

// Dispatch phases. Only "held" is non-executable.
const (
	ingressPhaseHeld      = "held"
	ingressPhaseQueued    = "queued"
	ingressPhaseExpired   = "expired"   // never reached the Agent; certainly not executed
	ingressPhaseFailed    = "failed"    // Agent acknowledged failure; host state needs receipt/inspection
	ingressPhaseUncertain = "uncertain" // dispatched, then expired without acknowledgement
)

func validIngressDispatchPhase(p string) bool {
	switch p {
	case ingressPhaseHeld, ingressPhaseQueued, ingressPhaseExpired, ingressPhaseFailed, ingressPhaseUncertain:
		return true
	}
	return false
}

func isOperationalIngressJob(j Job) bool {
	return (j.Type == agentjob.ActionPublicIngressApply || j.Type == agentjob.ActionPublicIngressRetire) && j.Params["ingress_protocol"] == ingressauthority.ProtocolV1
}

// operationalIngressJobParams derives the exact signed parameters from the
// reviewed immutable command; nothing is taken from desired state or callers.
func operationalIngressJobParams(cmd ingressauthority.CommandV1) map[string]string {
	b := cmd.Binding
	p := map[string]string{
		"tunnel_id": b.Spec.TunnelID, "public_port": strconv.Itoa(int(b.Port)), "local_target": b.LocalTarget,
		"generation": strconv.FormatUint(b.Spec.Generation, 10), "manifest_digest": b.Proof.ManifestSHA256,
		"listener_id": b.ListenerID, "firewall_id": b.FirewallID,
		"ingress_protocol": ingressauthority.ProtocolV1, "operation_id": b.OperationID, "lease_id": b.LeaseID,
		"plan_sha256": b.PlanSHA256, "config_sha256": b.Proof.ConfigSHA256, "listen_endpoint": b.ListenEndpoint,
		"transport": b.Transport,
	}
	if b.Spec.InstanceID != "" {
		p["instance_id"] = b.Spec.InstanceID
	}
	return p
}

func operationalIngressJob(cmd ingressauthority.CommandV1, now time.Time) (Job, error) {
	// agentjob.Sign truncates to whole seconds and the Agent compares the
	// signed window with the stored command exactly, so refuse anything else.
	if !cmd.IssuedAt.Equal(cmd.IssuedAt.UTC().Truncate(time.Second)) || !cmd.ExpiresAt.Equal(cmd.ExpiresAt.UTC().Truncate(time.Second)) {
		return Job{}, errors.New("operational ingress command window must be whole UTC seconds")
	}
	params := operationalIngressJobParams(cmd)
	signed := agentjob.Job{SchemaVersion: agentjob.SchemaVersion, JobID: cmd.JobID, NodeID: cmd.Binding.NodeID, Action: cmd.Action, Params: params, IssuedAt: cmd.IssuedAt, ExpiresAt: cmd.ExpiresAt}
	if err := signed.Validate(); err != nil {
		return Job{}, err
	}
	return Job{ID: cmd.JobID, Type: cmd.Action, NodeID: cmd.Binding.NodeID, Params: params, Status: "queued", CreatedAt: now.UTC(), UpdatedAt: now.UTC()}, nil
}

func sameIngressJobIdentity(old, want Job) bool {
	return old.ID == want.ID && old.Type == want.Type && old.NodeID == want.NodeID && reflect.DeepEqual(old.Params, want.Params)
}

// operationalIngressContextLocked rechecks the complete reviewed identity: the
// staged command, the active reviewed tunnel and an enrolled IR node.
func (s *Store) operationalIngressContextLocked(tunnelID string) (PublicIngressOperation, error) {
	op, ok := s.st.PublicIngressOperations[tunnelID]
	if !ok || op.Status != PublicIngressPending || op.Command == nil || op.Dispatch == nil {
		return op, errors.New("no staged and held ingress command available")
	}
	t, ok := s.st.Tunnels[tunnelID]
	if !ok || t.Phase != TunnelActive || t.PlanHash != op.ReviewedPlanHash || t.ClientPort != op.Spec.PublicPort || t.IRNode != op.Spec.IRNode || t.RouteListen != op.Spec.LocalTarget {
		return op, errors.New("reviewed tunnel changed")
	}
	node, ok := s.st.Nodes[op.Spec.IRNode]
	if !ok || node.Revoked || node.AgentTokenHash == "" || (node.Role != "worker" && node.Role != "master") {
		return op, errors.New("IR node is not enrolled or authorized")
	}
	cmd := *op.Command
	d := *op.Dispatch
	b := cmd.Binding
	if cmd.Action != agentjob.ActionPublicIngressApply || b.OperationID != op.OperationID || b.LeaseID != op.LeaseID || b.PlanSHA256 != op.ReviewedPlanHash || b.NodeID != op.Spec.IRNode || int(b.ConfigPort) != op.Spec.PublicPort || int(b.Port) != op.Spec.PublicPort ||
		d.JobID != cmd.JobID || d.OperationID != op.OperationID || d.LeaseID != op.LeaseID {
		return op, errors.New("staged command identity mismatch")
	}
	return op, nil
}

// EnqueueOperationalIngress queues the signed typed Apply job derived from the
// reviewed staged command, behind the closed-by-default authority gate. It is
// retry-safe: an identical queued job is a no-op; anything else that already
// consumed the immutable job identity (expired, failed, answered) is refused.
func (s *Store) EnqueueOperationalIngress(tunnelID string) error {
	return s.enqueueOperationalIngress(tunnelID, time.Now().UTC())
}

func (s *Store) enqueueOperationalIngress(tunnelID string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ingressEnqueueGateLocked(tunnelID, now); err != nil {
		return err
	}
	op, err := s.operationalIngressContextLocked(tunnelID)
	if err != nil {
		return err
	}
	cmd := *op.Command
	want, err := operationalIngressJob(cmd, now)
	if err != nil {
		return err
	}
	switch op.Dispatch.Phase {
	case ingressPhaseQueued:
		if old, ok := s.st.Jobs[cmd.JobID]; ok && sameIngressJobIdentity(old, want) {
			return nil
		}
		return errors.New("queued ingress job is missing or differs; replay refused")
	case ingressPhaseHeld:
	default:
		return errors.New("ingress dispatch already consumed; replay refused")
	}
	if op.Receipt != nil {
		return errors.New("ingress command already answered")
	}
	if _, exists := s.st.Jobs[cmd.JobID]; exists {
		return errors.New("ingress command JobID collides with ordinary BCC queue")
	}
	before, err := cloneState(s.st)
	if err != nil {
		return err
	}
	if err := cmd.Validate(now); err != nil {
		// The signed window has passed: compensate durably so it can never be
		// signed later, and report the expiry.
		d := *op.Dispatch
		d.Phase = ingressPhaseExpired
		op.Dispatch = &d
		s.st.PublicIngressOperations[tunnelID] = op
		if saveErr := s.saveLocked(); saveErr != nil {
			s.st = before
			return saveErr
		}
		return errors.New("staged ingress command outside its signed window")
	}
	d := *op.Dispatch
	d.Phase = ingressPhaseQueued
	op.Dispatch = &d
	s.st.PublicIngressOperations[tunnelID] = op
	s.st.Jobs[want.ID] = want
	if _, err := s.enqueueSecurityAuditLocked(AuditEntry{Timestamp: now, Actor: "bcc", Action: "public_ingress.apply.enqueue", Target: tunnelID, Outcome: "success",
		Details: map[string]any{"job_id": want.ID, "ir_node": want.NodeID, "operation_id": cmd.Binding.OperationID, "plan_hash": cmd.Binding.PlanSHA256}}); err != nil {
		s.st = before
		return err
	}
	if err := s.saveLocked(); err != nil {
		s.st = before
		return err
	}
	return nil
}

// EnqueueOperationalIngressRetire queues a NEW signed Retire job with the same
// binding. It requires an exact applied, ownership-verified Agent receipt for
// the Apply command, so Retire never rests on desired state or a missing map.
func (s *Store) EnqueueOperationalIngressRetire(tunnelID string) error {
	return s.enqueueOperationalIngressRetire(tunnelID, time.Now().UTC())
}

func (s *Store) enqueueOperationalIngressRetire(tunnelID string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ingressEnqueueGateLocked(tunnelID, now); err != nil {
		return err
	}
	op, err := s.operationalIngressContextLocked(tunnelID)
	if err != nil {
		return err
	}
	apply := *op.Command
	r := op.Receipt
	if op.Dispatch.Phase != ingressPhaseQueued || r == nil || r.Phase != "applied" || !r.OwnershipVerified || r.JobID != apply.JobID || r.Action != apply.Action || r.Binding != apply.Binding {
		return errors.New("retire requires an exact applied ownership-verified receipt")
	}
	if op.RetireReceipt != nil {
		return errors.New("ingress retire already answered")
	}
	if op.RetireCommand != nil {
		old := *op.RetireCommand
		if job, ok := s.st.Jobs[old.JobID]; ok && job.Status != "failed" {
			want, err := operationalIngressJob(old, now)
			if err != nil || !sameIngressJobIdentity(job, want) {
				return errors.New("queued retire job differs; replay refused")
			}
			return nil
		}
		// Previous retire failed/expired: a fresh JobID is required below.
	}
	jobID, err := newIngressIdentity("retire-")
	if err != nil {
		return err
	}
	cmd := ingressauthority.CommandV1{IngressProtocol: ingressauthority.ProtocolV1, JobID: jobID, Action: agentjob.ActionPublicIngressRetire, IssuedAt: now.UTC().Truncate(time.Second).Add(-time.Second), ExpiresAt: now.UTC().Truncate(time.Second).Add(jobValidity), Binding: apply.Binding}
	if err := cmd.Validate(now); err != nil {
		return err
	}
	want, err := operationalIngressJob(cmd, now)
	if err != nil {
		return err
	}
	for _, other := range s.st.PublicIngressOperations {
		if (other.Command != nil && other.Command.JobID == jobID) || (other.RetireCommand != nil && other.RetireCommand.JobID == jobID) {
			return errors.New("retire JobID collision")
		}
	}
	if _, exists := s.st.Jobs[jobID]; exists {
		return errors.New("retire JobID collides with ordinary BCC queue")
	}
	before, err := cloneState(s.st)
	if err != nil {
		return err
	}
	op.RetireCommand = &cmd
	s.st.PublicIngressOperations[tunnelID] = op
	s.st.Jobs[want.ID] = want
	if _, err := s.enqueueSecurityAuditLocked(AuditEntry{Timestamp: now, Actor: "bcc", Action: "public_ingress.retire.enqueue", Target: tunnelID, Outcome: "success",
		Details: map[string]any{"job_id": want.ID, "ir_node": want.NodeID, "operation_id": cmd.Binding.OperationID}}); err != nil {
		s.st = before
		return err
	}
	if err := s.saveLocked(); err != nil {
		s.st = before
		return err
	}
	return nil
}

// operationalIngressWindowLocked returns the immutable signed command of an
// operational job from persisted state, never from the mutable job record.
func (s *Store) operationalIngressWindowLocked(j Job) (ingressauthority.CommandV1, bool) {
	op, ok := s.st.PublicIngressOperations[j.Params["tunnel_id"]]
	if !ok {
		return ingressauthority.CommandV1{}, false
	}
	if op.Command != nil && op.Command.JobID == j.ID && j.Type == agentjob.ActionPublicIngressApply {
		return *op.Command, true
	}
	if op.RetireCommand != nil && op.RetireCommand.JobID == j.ID && j.Type == agentjob.ActionPublicIngressRetire {
		return *op.RetireCommand, true
	}
	return ingressauthority.CommandV1{}, false
}

// settleOperationalIngressJobLocked fails an operational job and compensates
// the apply dispatch phase. The caller saves.
func (s *Store) settleOperationalIngressJobLocked(j Job, phase, message string, now time.Time) {
	j.Status, j.Message, j.UpdatedAt = "failed", message, now
	s.st.Jobs[j.ID] = j
	if op, ok := s.st.PublicIngressOperations[j.Params["tunnel_id"]]; ok && op.Dispatch != nil && op.Dispatch.JobID == j.ID && op.Dispatch.Phase == ingressPhaseQueued {
		d := *op.Dispatch
		d.Phase = phase
		op.Dispatch = &d
		s.st.PublicIngressOperations[j.Params["tunnel_id"]] = op
	}
}

// ackOperationalIngressLocked handles an Agent job acknowledgement. An ack is
// never proof of a host change: success leaves everything PENDING until an
// exact authenticated receipt and independent verification exist.
func (s *Store) ackOperationalIngressLocked(j Job, status, message string, now time.Time) error {
	if j.Status != "dispatched" {
		return errors.New("ingress acknowledgement requires dispatched job")
	}
	before, err := cloneState(s.st)
	if err != nil {
		return err
	}
	cmd, ok := s.operationalIngressWindowLocked(j)
	expired := !ok || !now.Before(cmd.ExpiresAt)
	switch {
	case expired:
		s.settleOperationalIngressJobLocked(j, ingressPhaseUncertain, "operational ingress job expired before acknowledgement; host state uncertain", now)
	case status == "failed":
		s.settleOperationalIngressJobLocked(j, ingressPhaseFailed, "operational ingress job failed", now)
	default:
		j.Status, j.Message, j.UpdatedAt = "succeeded", message, now
		s.st.Jobs[j.ID] = j
	}
	if err := s.saveLocked(); err != nil {
		s.st = before
		return err
	}
	if expired {
		return errors.New("operational ingress job expired")
	}
	return nil
}

// signableOperationalIngress returns the persisted reviewed command a queued
// operational job may be signed from. Any drift refuses signing. Inside a pull
// the caller already holds the store lock and the commands are a snapshot.
func (s *Store) signableOperationalIngress(j Job, now time.Time) (ingressauthority.CommandV1, error) {
	if !isOperationalIngressJob(j) {
		return ingressauthority.CommandV1{}, errors.New("not an operational ingress job")
	}
	var cmd ingressauthority.CommandV1
	var ok bool
	if snapshot := s.signingCmds.Load(); snapshot != nil {
		cmd, ok = (*snapshot)[j.ID]
	} else {
		s.mu.Lock()
		cmd, ok = s.operationalIngressWindowLocked(j)
		s.mu.Unlock()
	}
	if !ok || cmd.Validate(now) != nil || cmd.Binding.NodeID != j.NodeID || !reflect.DeepEqual(operationalIngressJobParams(cmd), j.Params) {
		return ingressauthority.CommandV1{}, errors.New("operational ingress job differs from persisted reviewed command or is outside its window")
	}
	return cmd, nil
}
