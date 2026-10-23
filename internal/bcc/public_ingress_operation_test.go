package bcc

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

func canonicalOperationProof(t *testing.T, s *Store, spec PublicIngressSpec, generation uint64) PublicIngressExpectedProof {
	t.Helper()
	manifest, err := ingressmanifest.Preview(publicIngressManifestSpec(spec, s.st.Tunnels[spec.TunnelID].InstanceID, generation))
	if err != nil {
		t.Fatal(err)
	}
	return PublicIngressExpectedProof{Generation: generation, ManifestDigest: manifest.ManifestSHA256, ListenerID: fmt.Sprintf("listener-%d", generation), FirewallID: fmt.Sprintf("firewall-%d", generation)}
}

func TestPublicIngressOperationIntentIsDurableAndFenced(t *testing.T) {
	s, spec := reservationStore(t)
	if _, err := s.ReservePublicIngress(spec, time.Now(), ingressAudit()); err != nil {
		t.Fatal(err)
	}
	proof := canonicalOperationProof(t, s, spec, 1)
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
	backup := state{Tunnels: reopened.st.Tunnels, PublicIngressReservations: reopened.st.PublicIngressReservations, PublicIngressOperations: map[string]PublicIngressOperation{spec.TunnelID: {Spec: spec, Proof: other, Status: PublicIngressPending, CreatedAt: time.Now()}}}
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
	proof := canonicalOperationProof(t, s, spec, 1)
	if _, err := s.RecordPublicIngressOperationIntent(spec, proof, time.Now(), ingressAudit()); err == nil {
		t.Fatal("unreserved operation accepted")
	}
	if _, err := s.ReservePublicIngress(spec, time.Now(), ingressAudit()); err != nil {
		t.Fatal(err)
	}
	wrong := proof
	wrong.ManifestDigest = strings.Repeat("a", 64)
	if _, err := s.RecordPublicIngressOperationIntent(spec, wrong, time.Now(), ingressAudit()); err == nil {
		t.Fatal("noncanonical manifest digest accepted")
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
	proof := canonicalOperationProof(t, s, spec, 1)
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

func TestPublicIngressRestoreRejectsNoncanonicalOperationProof(t *testing.T) {
	s, spec := reservationStore(t)
	if _, err := s.ReservePublicIngress(spec, time.Now(), ingressAudit()); err != nil {
		t.Fatal(err)
	}
	proof := canonicalOperationProof(t, s, spec, 1)
	if _, err := s.RecordPublicIngressOperationIntent(spec, proof, time.Now(), ingressAudit()); err != nil {
		t.Fatal(err)
	}
	op := s.st.PublicIngressOperations[spec.TunnelID]
	restored := state{Tunnels: s.st.Tunnels, PublicIngressReservations: s.st.PublicIngressReservations, PublicIngressOperations: map[string]PublicIngressOperation{spec.TunnelID: op}}
	changed := op
	changed.Proof.ManifestDigest = strings.Repeat("b", 64)
	restored.PublicIngressOperations[spec.TunnelID] = changed
	if err := mergePublicIngressOperations(&restored, state{}); err == nil {
		t.Fatal("tampered digest accepted")
	}
	restored.PublicIngressOperations[spec.TunnelID] = op
	delete(restored.Tunnels, spec.TunnelID)
	if err := mergePublicIngressOperations(&restored, state{}); err == nil {
		t.Fatal("missing owning tunnel accepted")
	}
}
