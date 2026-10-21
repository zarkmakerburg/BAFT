package bcc

import (
	"strings"
	"testing"
	"time"
)

func TestPublicIngressOperationIntentIsDurableAndFenced(t *testing.T) {
	s, spec := reservationStore(t)
	if _, err := s.ReservePublicIngress(spec, time.Now(), ingressAudit()); err != nil {
		t.Fatal(err)
	}
	proof := PublicIngressExpectedProof{Generation: 1, ManifestDigest: strings.Repeat("a", 64), ListenerID: "listener-1", FirewallID: "firewall-1"}
	if _, err := s.RecordPublicIngressOperationIntent(spec, proof, time.Now(), ingressAudit()); err != nil {
		t.Fatal(err)
	}
	first := s.st.PublicIngressOperations[spec.TunnelID]
	if first.Status != PublicIngressPending || first.Spec != spec || first.Proof != proof {
		t.Fatalf("unexpected cursor: %+v", first)
	}
	auditCount := len(s.PendingSecurityAuditIntents())
	if same, err := s.RecordPublicIngressOperationIntent(spec, proof, time.Now().Add(time.Hour), ingressAudit()); err != nil || same != first || len(s.PendingSecurityAuditIntents()) != auditCount {
		t.Fatalf("retry mutated cursor: %+v %v", same, err)
	}
	other := proof
	other.Generation++
	if _, err := s.RecordPublicIngressOperationIntent(spec, other, time.Now(), ingressAudit()); err == nil {
		t.Fatal("new generation replaced active cursor")
	}
	reopened, err := OpenStore(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.st.PublicIngressOperations[spec.TunnelID]; got != first {
		t.Fatalf("restart lost cursor: %+v", got)
	}
	backup := state{PublicIngressReservations: reopened.st.PublicIngressReservations, PublicIngressOperations: map[string]PublicIngressOperation{spec.TunnelID: {Spec: spec, Proof: other, Status: PublicIngressPending, CreatedAt: time.Now()}}}
	if err := mergePublicIngressOperations(&backup, reopened.st); err == nil {
		t.Fatal("restore replaced current generation")
	}
	backup.PublicIngressOperations = nil
	if err := mergePublicIngressOperations(&backup, reopened.st); err != nil || backup.PublicIngressOperations[spec.TunnelID] != first {
		t.Fatalf("restore dropped current cursor: %v", err)
	}
}

func TestPublicIngressOperationRequiresReservationAndProof(t *testing.T) {
	s, spec := reservationStore(t)
	proof := PublicIngressExpectedProof{Generation: 1, ManifestDigest: strings.Repeat("a", 64), ListenerID: "listener-1", FirewallID: "firewall-1"}
	if _, err := s.RecordPublicIngressOperationIntent(spec, proof, time.Now(), ingressAudit()); err == nil {
		t.Fatal("unreserved operation accepted")
	}
	if _, err := s.ReservePublicIngress(spec, time.Now(), ingressAudit()); err != nil {
		t.Fatal(err)
	}
	proof.FirewallID = proof.ListenerID
	if _, err := s.RecordPublicIngressOperationIntent(spec, proof, time.Now(), ingressAudit()); err == nil {
		t.Fatal("invalid resource proof accepted")
	}
	if len(s.st.PublicIngressOperations) != 0 {
		t.Fatal("invalid intent persisted")
	}
}

func TestPublicIngressOperationSaveFailureRollsBackCursorAndAudit(t *testing.T) {
	s, spec := reservationStore(t)
	if _, err := s.ReservePublicIngress(spec, time.Now(), ingressAudit()); err != nil {
		t.Fatal(err)
	}
	proof := PublicIngressExpectedProof{Generation: 1, ManifestDigest: strings.Repeat("a", 64), ListenerID: "listener-1", FirewallID: "firewall-1"}
	before := len(s.PendingSecurityAuditIntents())
	path := s.path
	s.path = t.TempDir()
	if _, err := s.RecordPublicIngressOperationIntent(spec, proof, time.Now(), ingressAudit()); err == nil {
		t.Fatal("failed state save accepted")
	}
	s.path = path
	if len(s.st.PublicIngressOperations) != 0 || len(s.PendingSecurityAuditIntents()) != before {
		t.Fatal("failed state save changed cursor or audit")
	}
}
