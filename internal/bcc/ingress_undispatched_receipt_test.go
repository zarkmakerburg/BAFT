package bcc

import (
	"testing"
	"time"
)

// Counterpart of hq's "undispatched job accepted positive receipt" assertion,
// expressed in this branch's fail-closed terms: an applied receipt may be
// recorded as inert evidence, but for a command that was never queued it can
// neither make the ingress Ready (even with a matching independent
// observation) nor unlock Retire.
func TestAppliedReceiptForUndispatchedCommandNeverReadyNorRetirable(t *testing.T) {
	openIngressGateForTest(t)
	s, spec, cmd, token := heldIngress(t) // staged + HELD, never enqueued
	now := time.Now().UTC()
	if err := s.RecordIngressAgentReceipt(appliedReceipt(cmd), token, now); err != nil {
		t.Fatal(err)
	}
	obs := &IndependentIngressObservation{Binding: cmd.Binding, ObservedAt: now, ListenerEndpoint: cmd.Binding.ListenEndpoint,
		SocketIdentity: "socket:12345", ChainHandle: 3, RuleHandle: 9, TCPPayloadVerified: true}
	if r := s.EvaluateIngressReadiness(spec.TunnelID, obs, now); r.Ready || r.Verified || r.Card != nil {
		t.Fatalf("receipt for a never-dispatched command reported ready: %+v", r)
	}
	if st := s.IngressOperationalState(spec.TunnelID, obs, now); st == nil || st.Ready || st.State != "held" {
		t.Fatalf("state overstated: %+v", st)
	}
	if err := s.EnqueueOperationalIngressRetire(spec.TunnelID); err == nil {
		t.Fatal("retire queued for a never-dispatched command")
	}
}
