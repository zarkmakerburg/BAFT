package bcc

import (
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
)

// A failed Retire receipt says cleanup is NOT done: it must not make the
// Retire terminal. A NEW signed Retire (fresh JobID) resumes it, and only a
// completed ("retired") Retire is final.
func TestOperationalIngressFailedRetireReceiptIsResumableByNewSignedRetire(t *testing.T) {
	openIngressGateForTest(t)
	s, spec, cmd, token := heldIngress(t)
	if err := s.EnqueueOperationalIngress(spec.TunnelID); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	op := s.st.PublicIngressOperations[spec.TunnelID]
	applied := appliedReceipt(cmd)
	op.Receipt = &applied
	s.st.PublicIngressOperations[spec.TunnelID] = op
	s.mu.Unlock()
	if err := s.EnqueueOperationalIngressRetire(spec.TunnelID); err != nil {
		t.Fatal(err)
	}
	first, _ := s.GetPublicIngressOperation(spec.TunnelID)
	failed := ingressauthority.ReceiptV1{SchemaVersion: 1, JobID: first.RetireCommand.JobID, Action: "public_ingress_retire", Binding: cmd.Binding,
		Phase: "failed", FailureCause: "operational ingress outcome uncertain: nft delete failed", RollbackStatus: "reconciliation_required"}
	if err := s.RecordIngressAgentReceipt(failed, token, time.Now()); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	j := s.st.Jobs[first.RetireCommand.JobID]
	j.Status = "failed"
	s.st.Jobs[first.RetireCommand.JobID] = j
	s.mu.Unlock()
	withFailed, _ := s.GetPublicIngressOperation(spec.TunnelID)
	if !retireNeeded(withFailed) {
		t.Fatal("a failed Retire must still count as retire-needed")
	}
	if err := s.EnqueueOperationalIngressRetire(spec.TunnelID); err != nil {
		t.Fatalf("new signed Retire after a failed one: %v", err)
	}
	second, _ := s.GetPublicIngressOperation(spec.TunnelID)
	if second.RetireCommand.JobID == first.RetireCommand.JobID || second.RetireReceipt != nil {
		t.Fatalf("resume must mint a fresh JobID and clear the superseded receipt: %+v", second)
	}
	if err := validatePersistedIngressOperations(s.st); err != nil {
		t.Fatal(err)
	}
	done := ingressauthority.ReceiptV1{SchemaVersion: 1, JobID: second.RetireCommand.JobID, Action: "public_ingress_retire", Binding: cmd.Binding, Phase: "retired"}
	if err := s.RecordIngressAgentReceipt(done, token, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.EnqueueOperationalIngressRetire(spec.TunnelID); err == nil {
		t.Fatal("a completed Retire must be terminal")
	}
}
