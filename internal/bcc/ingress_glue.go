package bcc

import (
	"errors"
	"strings"
	"sync/atomic"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// ingressGlueDisabled exists only so in-package tests that stage by hand can
// opt out of the production glue. It has no production setter.
var ingressGlueDisabled atomic.Bool

// ingressHeldCommandWindow is how long a staged (held) command stays signable
// while the production gate is closed. Whole UTC seconds.
const ingressHeldCommandWindow = 24 * time.Hour

// stageReviewedIngressOnActivationLocked runs inside AdvanceTunnels, under the
// store lock and before its single save, when a tunnel becomes ACTIVE. It
// never creates a Job: the command is only STAGED and the dispatch HELD. Any
// failure leaves state untouched (fail-closed).
func (s *Store) stageReviewedIngressOnActivationLocked(tunnelID string, now time.Time) {
	if ingressGlueDisabled.Load() {
		return
	}
	_ = s.stageReviewedIngressLocked(tunnelID, now)
}

// stageReviewedIngressLocked records the reviewed operation, the immutable
// Apply command and the held dispatch intent without saving; on any failure
// the in-memory state is restored. Idempotent once fully staged.
func (s *Store) stageReviewedIngressLocked(tunnelID string, now time.Time) error {
	t, ok := s.st.Tunnels[tunnelID]
	if !ok || t.Phase != TunnelActive || t.ClientPort == 0 || t.PlanHash == "" {
		return errors.New("not a reviewed active tunnel with a client port")
	}
	if op, exists := s.st.PublicIngressOperations[tunnelID]; exists && op.Command != nil && op.Dispatch != nil {
		return nil
	}
	spec, err := PlanPublicIngress(t.IRNode, t.ID, t.ClientPort, t.RouteListen)
	if err != nil {
		return err
	}
	generation := s.nextIngressGenerationLocked(spec.IRNode, spec.PublicPort)
	manifest := publicIngressManifestSpec(spec, t.instanceForNode(t.IRNode), generation)
	preview, err := ingressmanifest.Preview(manifest)
	if err != nil {
		return err
	}
	proof := PublicIngressExpectedProof{Generation: generation, ManifestDigest: preview.ManifestSHA256, ListenerID: "listener-" + t.ID, FirewallID: "firewall-" + t.ID}
	if !proof.valid() {
		return errors.New("noncanonical ingress resource identity")
	}
	before, err := cloneState(s.st)
	if err != nil {
		return err
	}
	fail := func(err error) error { s.st = before; return err }
	op, err := s.recordReviewedPublicIngressOperationLocked(spec, t.PlanHash, proof, now, AuditEntry{Timestamp: now, Actor: "bcc"}, false)
	if err != nil {
		return fail(err)
	}
	jobID, err := newIngressIdentity("apply-")
	if err != nil {
		return fail(err)
	}
	issued := now.UTC().Truncate(time.Second).Add(-time.Second)
	cmd := ingressauthority.CommandV1{
		IngressProtocol: ingressauthority.ProtocolV1, JobID: jobID, Action: agentjob.ActionPublicIngressApply,
		IssuedAt: issued, ExpiresAt: issued.Add(ingressHeldCommandWindow),
		Binding: ingressauthority.BindingV1{
			NodeID: spec.IRNode, OperationID: op.OperationID, LeaseID: op.LeaseID,
			Port: uint16(spec.PublicPort), ConfigPort: uint16(spec.PublicPort),
			ListenEndpoint: manifest.Config.Listen, LocalTarget: manifest.Config.Target, Transport: "dedicated",
			PlanSHA256: t.PlanHash, ListenerID: proof.ListenerID, FirewallID: proof.FirewallID, Spec: manifest, Proof: preview,
		},
	}
	if err := s.stageReviewedIngressCommandLocked(tunnelID, cmd, now, false); err != nil {
		return fail(err)
	}
	if _, err := s.stageHeldIngressDispatchLocked(tunnelID, now, false); err != nil {
		return fail(err)
	}
	return nil
}

// restageExpiredUndispatchedIngressLocked renews only an expired command whose
// durable dispatch phase proves it was never queued and whose JobID has never
// entered the BCC queue. Fresh operation, lease and Job identities prevent a
// stale signed command from being confused with the new attempt. Any evidence
// of prior dispatch, receipt, or resource lifecycle fails closed.
func (s *Store) restageExpiredUndispatchedIngressLocked(tunnelID string, now time.Time) error {
	op, ok := s.st.PublicIngressOperations[tunnelID]
	if !ok || op.Dispatch == nil || op.Dispatch.Phase != ingressPhaseExpired {
		return nil
	}
	if op.Command == nil || op.Receipt != nil || op.RetireCommand != nil || op.RetireReceipt != nil || op.Status != PublicIngressPending {
		return errors.New("expired ingress is not an undispatched held Apply")
	}
	old := *op.Command
	if op.Dispatch.JobID != old.JobID || op.Dispatch.OperationID != op.OperationID || op.Dispatch.LeaseID != op.LeaseID ||
		now.Before(old.ExpiresAt) || now.Before(old.IssuedAt) {
		return errors.New("expired ingress dispatch identity or expiry is inconsistent")
	}
	if _, exists := s.st.Jobs[old.JobID]; exists {
		return errors.New("expired ingress JobID exists in queue; safe restage refused")
	}
	t, ok := s.st.Tunnels[tunnelID]
	if !ok || t.Phase != TunnelActive || t.PlanHash != op.ReviewedPlanHash || t.ClientPort != op.Spec.PublicPort ||
		t.IRNode != op.Spec.IRNode || t.RouteListen != op.Spec.LocalTarget || old.Action != agentjob.ActionPublicIngressApply ||
		old.Validate(old.IssuedAt) != nil || old.Binding.OperationID != op.OperationID || old.Binding.LeaseID != op.LeaseID ||
		old.Binding.PlanSHA256 != op.ReviewedPlanHash || old.Binding.NodeID != op.Spec.IRNode ||
		int(old.Binding.Port) != op.Spec.PublicPort || int(old.Binding.ConfigPort) != op.Spec.PublicPort ||
		old.Binding.Spec.TunnelID != tunnelID || old.Binding.Spec.InstanceID != t.instanceForNode(t.IRNode) ||
		old.Binding.Spec.Generation != op.Proof.Generation || old.Binding.ListenerID != op.Proof.ListenerID ||
		old.Binding.FirewallID != op.Proof.FirewallID || old.Binding.Proof.ManifestSHA256 != op.Proof.ManifestDigest {
		return errors.New("expired ingress no longer matches the active reviewed plan")
	}
	expectedSpec := publicIngressManifestSpec(op.Spec, t.instanceForNode(t.IRNode), op.Proof.Generation)
	expectedProof, err := ingressmanifest.Preview(expectedSpec)
	if err != nil || old.Binding.Spec != expectedSpec || old.Binding.Proof != expectedProof ||
		old.Binding.ListenEndpoint != expectedSpec.Config.Listen || old.Binding.LocalTarget != expectedSpec.Config.Target || old.Binding.Transport != "dedicated" {
		return errors.New("expired ingress command is not the canonical reviewed manifest")
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
	cmd := old
	cmd.JobID = newJobID
	cmd.IssuedAt = now.UTC().Truncate(time.Second).Add(-time.Second)
	cmd.ExpiresAt = cmd.IssuedAt.Add(ingressHeldCommandWindow)
	cmd.Binding.OperationID = newOperationID
	cmd.Binding.LeaseID = newLeaseID
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
	op.OperationID, op.LeaseID = newOperationID, newLeaseID
	op.Command = &cmd
	op.Dispatch = &IngressDispatchIntent{JobID: newJobID, OperationID: newOperationID, LeaseID: newLeaseID, Phase: ingressPhaseHeld, CreatedAt: now.UTC()}
	s.st.PublicIngressOperations[tunnelID] = op
	if _, err := s.enqueueSecurityAuditLocked(AuditEntry{Timestamp: now, Actor: "bcc", Action: "public_ingress.apply.restage", Target: tunnelID, Outcome: "success",
		Details: map[string]any{"superseded_job_id": old.JobID, "superseded_operation_id": old.Binding.OperationID, "superseded_lease_id": old.Binding.LeaseID,
			"job_id": newJobID, "operation_id": newOperationID, "lease_id": newLeaseID, "reason": "expired while durably held and never queued"}}); err != nil {
		s.st = before
		return err
	}
	if err := s.saveLocked(); err != nil {
		s.st = before
		return err
	}
	return nil
}

// RetryHeldOperationalIngress re-attempts the enqueue of every HELD Apply for
// an IR node, after an authenticated agent poll recorded its capabilities. The
// production gate decides; a closed gate or any refusal leaves it held.
func (s *Store) RetryHeldOperationalIngress(nodeID string) {
	s.mu.Lock()
	var ids []string
	for id, op := range s.st.PublicIngressOperations {
		if op.Spec.IRNode == nodeID && op.Dispatch != nil && op.Dispatch.Phase == ingressPhaseHeld && op.Command != nil && op.Receipt == nil {
			ids = append(ids, id)
		}
	}
	s.mu.Unlock()
	for _, id := range ids {
		_ = s.EnqueueOperationalIngress(id)
	}
}

// retireNeeded reports whether the operation has an applied, ownership-verified
// ingress that has no Retire receipt yet.
func retireNeeded(op PublicIngressOperation) bool {
	r := op.Receipt
	return op.Command != nil && op.Dispatch != nil && r != nil && r.Phase == "applied" && r.OwnershipVerified && (op.RetireReceipt == nil || op.RetireReceipt.Phase != "retired")
}

// PrecheckOperationalIngressRetire is read-only. needed=false: nothing applied
// to retire. needed=true with nil error: a Retire can be queued right now.
func (s *Store) PrecheckOperationalIngressRetire(tunnelID string) (needed bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	op, ok := s.st.PublicIngressOperations[tunnelID]
	if !ok || !retireNeeded(op) {
		return false, nil
	}
	if op.RetireCommand != nil {
		if j, ok := s.st.Jobs[op.RetireCommand.JobID]; ok && j.Status != "failed" {
			return true, nil // already queued; enqueue is a no-op
		}
	}
	if err := s.ingressEnqueueGateLocked(tunnelID, time.Now().UTC()); err != nil {
		return true, err
	}
	if _, err := s.operationalIngressContextLocked(tunnelID); err != nil {
		return true, err
	}
	return true, nil
}

// RetireAppliedOperationalIngress queues a Retire (new JobID) when the tunnel
// has an applied ingress. It must run while the tunnel is still ACTIVE.
func (s *Store) RetireAppliedOperationalIngress(tunnelID string) (queued bool, err error) {
	needed, err := s.PrecheckOperationalIngressRetire(tunnelID)
	if !needed || err != nil {
		return false, err
	}
	if err := s.EnqueueOperationalIngressRetire(tunnelID); err != nil {
		return false, err
	}
	return true, nil
}

// AbandonQueuedOperationalRetire compensates a Retire that was queued for a
// user action that then failed: a Retire still undispatched is failed so it can
// never run; an already dispatched one is left to the Agent.
func (s *Store) AbandonQueuedOperationalRetire(tunnelID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	op, ok := s.st.PublicIngressOperations[tunnelID]
	if !ok || op.RetireCommand == nil {
		return nil
	}
	j, ok := s.st.Jobs[op.RetireCommand.JobID]
	if !ok || j.Status != "queued" {
		return nil
	}
	before, err := cloneState(s.st)
	if err != nil {
		return err
	}
	s.settleOperationalIngressJobLocked(j, ingressPhaseFailed, "retire abandoned: the user action did not complete", time.Now().UTC())
	if err := s.saveLocked(); err != nil {
		s.st = before
		return err
	}
	return nil
}

// IngressOperationalState is the minimal per-tunnel view of the operational
// ingress. Ready is true only with fresh independent verification.
type IngressOperationalState struct {
	State  string `json:"state"` // held|queued|applied|blocked|failed|retiring|retired
	Reason string `json:"reason,omitempty"`
	Ready  bool   `json:"ready"`
	JobID  string `json:"job_id,omitempty"`
}

// IngressOperationalState derives the state from persisted facts only; nil
// when the tunnel has no staged operational ingress. obs may be nil.
func (s *Store) IngressOperationalState(tunnelID string, obs *IndependentIngressObservation, now time.Time) *IngressOperationalState {
	st := s.ingressOperationalStateBase(tunnelID, now)
	if st != nil && st.State == "applied" && obs != nil {
		if r := s.EvaluateIngressReadiness(tunnelID, obs, now); r.Ready {
			st.Ready, st.Reason = true, ""
		} else {
			st.Reason = r.Reason
		}
	}
	return st
}

func (s *Store) ingressOperationalStateBase(tunnelID string, now time.Time) *IngressOperationalState {
	s.mu.Lock()
	defer s.mu.Unlock()
	op, ok := s.st.PublicIngressOperations[tunnelID]
	if !ok || op.Command == nil || op.Dispatch == nil {
		return nil
	}
	st := &IngressOperationalState{JobID: op.Command.JobID}
	switch {
	case op.RetireReceipt != nil:
		st.State, st.JobID = "retired", op.RetireCommand.JobID
		st.Reason = "retire receipt recorded; absence not independently verified"
		if op.RetireReceipt.Phase != "applied" && op.RetireReceipt.Phase != "retired" {
			st.State, st.Reason = "failed", strings.TrimSpace(op.RetireReceipt.FailureCause)
		}
	case op.RetireCommand != nil:
		st.State, st.JobID = "retiring", op.RetireCommand.JobID
	case op.Dispatch.Phase == ingressPhaseHeld:
		st.State = "held"
		st.Reason = "waiting for the IR node agent to report " + IngressCapabilityOperationalV1
		if s.ingressEnqueueGateLocked(tunnelID, now) == nil {
			st.Reason = "gate open; enqueue pending"
		}
	case op.Dispatch.Phase == ingressPhaseExpired || op.Dispatch.Phase == ingressPhaseUncertain:
		st.State, st.Reason = "blocked", "signed window passed ("+op.Dispatch.Phase+"); operator review required"
	case op.Dispatch.Phase == ingressPhaseFailed || (op.Receipt != nil && op.Receipt.Phase != "applied"):
		st.State = "failed"
		if op.Receipt != nil {
			st.Reason = op.Receipt.FailureCause
		}
	case op.Receipt != nil:
		st.State, st.Reason = "applied", "awaiting independent verification"
	default:
		st.State = "queued"
	}
	return st
}
