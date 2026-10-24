package bcc

import (
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
)

// ingressObservationMaxAge bounds how stale an independent observation may be.
const ingressObservationMaxAge = 5 * time.Minute

// IndependentIngressObservation is evidence BCC obtained NOT from the Agent's
// own receipt (for example its own TCP probe through the public port plus a
// separately authenticated inspection). The receipt alone never suffices.
type IndependentIngressObservation struct {
	Binding            ingressauthority.BindingV1
	ObservedAt         time.Time
	ListenerEndpoint   string
	SocketIdentity     string
	ChainHandle        uint64
	RuleHandle         uint64
	TCPPayloadVerified bool
}

// IngressConfigCard is the only deployable client configuration BCC may show.
type IngressConfigCard struct {
	TunnelID   string `json:"tunnel_id"`
	IRNode     string `json:"ir_node"`
	ConfigPort uint16 `json:"config_port"`
	PublicPort uint16 `json:"public_port"`
	PlanSHA256 string `json:"plan_sha256"`
}

// IngressReadiness is derived on demand and never persisted, so desired state,
// a backup restore or a stale record can never carry a Ready claim.
type IngressReadiness struct {
	Verified bool               `json:"verified"`
	Ready    bool               `json:"ready"`
	Reason   string             `json:"reason,omitempty"`
	Card     *IngressConfigCard `json:"card,omitempty"`
}

func notReady(reason string) IngressReadiness { return IngressReadiness{Reason: reason} }

// EvaluateIngressReadiness reports Verified/Ready (and the config card) ONLY
// when all hold: the dispatch was actually queued, an exact authenticated
// Agent receipt for the stored Apply command reports applied with verified
// ownership and complete evidence, and a fresh independent observation agrees
// with that receipt on the same binding, socket and firewall handles and a
// real TCP payload round trip. Retire evidence turns it back off.
func (s *Store) EvaluateIngressReadiness(tunnelID string, obs *IndependentIngressObservation, now time.Time) IngressReadiness {
	s.mu.Lock()
	defer s.mu.Unlock()
	ev, reason := s.ingressReceiptEvidenceLocked(tunnelID)
	if reason != "" {
		return notReady(reason)
	}
	cmd, l, fw := ev.cmd, ev.listener, ev.fw
	if obs == nil {
		return IngressReadiness{Reason: "awaiting independent verification"}
	}
	if obs.Binding != cmd.Binding || obs.ObservedAt.IsZero() || obs.ObservedAt.After(now.Add(time.Minute)) || now.Sub(obs.ObservedAt) > ingressObservationMaxAge ||
		obs.ListenerEndpoint != l.Endpoint || obs.SocketIdentity != l.SocketIdentity || obs.ChainHandle != fw.ChainHandle || obs.RuleHandle != fw.RuleHandle || !obs.TCPPayloadVerified {
		return IngressReadiness{Reason: "independent evidence does not match the authenticated receipt"}
	}
	return IngressReadiness{Verified: true, Ready: true, Card: &IngressConfigCard{
		TunnelID: tunnelID, IRNode: cmd.Binding.NodeID, ConfigPort: cmd.Binding.ConfigPort, PublicPort: cmd.Binding.Port, PlanSHA256: cmd.Binding.PlanSHA256,
	}}
}

// ingressReceiptEvidence is the exact authenticated applied-receipt evidence
// that an independent observation must agree with.
type ingressReceiptEvidence struct {
	cmd      ingressauthority.CommandV1
	listener ingressauthority.ListenerEvidenceV1
	fw       ingressauthority.FirewallEvidenceV1
}

// ingressReceiptEvidenceLocked returns the evidence, or a non-empty reason why
// the dispatched command / receipt cannot support any readiness claim.
func (s *Store) ingressReceiptEvidenceLocked(tunnelID string) (ingressReceiptEvidence, string) {
	none := ingressReceiptEvidence{}
	op, ok := s.st.PublicIngressOperations[tunnelID]
	if !ok || op.Status != PublicIngressPending || op.Command == nil || op.Dispatch == nil || op.Dispatch.Phase != ingressPhaseQueued {
		return none, "no dispatched operational ingress command"
	}
	if op.RetireCommand != nil {
		return none, "retire in progress or completed"
	}
	if _, queued := s.st.Jobs[op.Command.JobID]; !queued {
		return none, "dispatched job missing"
	}
	cmd, r := *op.Command, op.Receipt
	if r == nil {
		return none, "no authenticated Agent receipt"
	}
	if r.SchemaVersion != 1 || r.JobID != cmd.JobID || r.Action != cmd.Action || r.Binding != cmd.Binding || r.Phase != "applied" || !r.OwnershipVerified || r.FailureCause != "" || r.RollbackStatus != "" {
		return none, "receipt is not an exact applied ownership-verified receipt"
	}
	l := r.ListenerEvidence
	if l.Endpoint != cmd.Binding.ListenEndpoint || l.SocketIdentity == "" || len(r.FirewallEvidence) == 0 {
		return none, "receipt evidence incomplete"
	}
	var fw *ingressauthority.FirewallEvidenceV1
	for i := range r.FirewallEvidence {
		e := &r.FirewallEvidence[i]
		if e.Table == "" || e.Chain == "" || e.OwnerTag == "" || e.ChainHandle == 0 || e.RuleHandle == 0 {
			return none, "receipt firewall evidence incomplete"
		}
		if fw == nil {
			fw = e
		}
	}
	return ingressReceiptEvidence{cmd: cmd, listener: l, fw: *fw}, ""
}
