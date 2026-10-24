package bcc

import (
	"testing"
	"time"
)

func TestReviewedIngressOperationAtomicallyPersistsDesiredReservationAndProof(t *testing.T) {
	s, spec := reservationStore(t)
	s.mu.Lock()
	tun := s.st.Tunnels[spec.TunnelID]
	tun.PlanHash = "reviewed-hash"
	tun.ClientPort = spec.PublicPort
	s.st.Tunnels[tun.ID] = tun
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()
	proof := canonicalOperationProof(t, s, spec, 1)
	op, err := s.RecordReviewedPublicIngressOperation(spec, "reviewed-hash", proof, time.Now(), ingressAudit())
	if err != nil {
		t.Fatal(err)
	}
	if op.Spec != spec || op.Proof != proof || op.Status != PublicIngressPending || op.ReviewedPlanHash != "reviewed-hash" {
		t.Fatalf("wrong cursor: %+v", op)
	}
	reopened, err := OpenStore(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.st.PublicIngressOperations[spec.TunnelID]; got != op {
		t.Fatalf("restart lost operation: %+v", got)
	}
	if got := reopened.st.PublicIngressReservations[spec.TunnelID]; got.Spec != spec {
		t.Fatalf("restart lost reservation: %+v", got)
	}
	if _, err := s.RecordReviewedPublicIngressOperation(spec, "wrong", proof, time.Now(), ingressAudit()); err == nil {
		t.Fatal("stale hash accepted")
	}
	wrong := proof
	wrong.ManifestDigest = "bad"
	if _, err := s.RecordReviewedPublicIngressOperation(spec, "reviewed-hash", wrong, time.Now(), ingressAudit()); err == nil {
		t.Fatal("invalid proof accepted")
	}
}

func TestReviewedIngressOperationRejectedSaveLeavesNoPartialReservation(t *testing.T) {
	s, spec := reservationStore(t)
	s.mu.Lock()
	tun := s.st.Tunnels[spec.TunnelID]
	tun.PlanHash = "reviewed-hash"
	tun.ClientPort = spec.PublicPort
	s.st.Tunnels[tun.ID] = tun
	s.mu.Unlock()
	proof := canonicalOperationProof(t, s, spec, 1)
	path := s.path
	s.path = t.TempDir()
	if _, err := s.RecordReviewedPublicIngressOperation(spec, "reviewed-hash", proof, time.Now(), ingressAudit()); err == nil {
		t.Fatal("unwritable state accepted")
	}
	s.path = path
	if len(s.st.PublicIngressReservations) != 0 || len(s.st.PublicIngressOperations) != 0 {
		t.Fatal("partial desired reservation or operation survived failed save")
	}
}

func TestReviewedOperationRestoreRejectsChangedPlanHashOrClientPort(t *testing.T) {
	s, spec := reservationStore(t)
	s.mu.Lock()
	tun := s.st.Tunnels[spec.TunnelID]
	tun.PlanHash = "reviewed-hash"
	tun.ClientPort = spec.PublicPort
	s.st.Tunnels[tun.ID] = tun
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()
	proof := canonicalOperationProof(t, s, spec, 1)
	if _, err := s.RecordReviewedPublicIngressOperation(spec, "reviewed-hash", proof, time.Now(), ingressAudit()); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Tunnel)
	}{
		{"different plan", func(t *Tunnel) { t.PlanHash = "replaced" }},
		{"different client port", func(t *Tunnel) { t.ClientPort++ }},
		{"non-active tunnel", func(t *Tunnel) { t.Phase = TunnelRollingBack }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, err := s.snapshotState()
			if err != nil {
				t.Fatal(err)
			}
			tun := snapshot.Tunnels[spec.TunnelID]
			tc.mutate(&tun)
			snapshot.Tunnels[spec.TunnelID] = tun
			if err := mergePublicIngressOperations(&snapshot, s.st); err == nil {
				t.Fatal("restore admitted operation detached from reviewed plan")
			}
		})
	}
}
