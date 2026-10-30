//go:build unix

package ingressauthority

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

func nextGen(t *testing.T, b BindingV1, gen uint64, fresh bool) BindingV1 {
	t.Helper()
	n := b
	n.Spec.Generation = gen
	p, err := ingressmanifest.Preview(n.Spec)
	if err != nil {
		t.Fatal(err)
	}
	n.Proof = p
	if fresh {
		n.OperationID, n.LeaseID, n.PlanSHA256 = "op-2", "lease-2", strings.Repeat("c", 64)
	}
	if err := n.Validate(); err != nil {
		t.Fatal(err)
	}
	return n
}

func seedReleased(t *testing.T, s *ReservationStore, b BindingV1, spent ...string) {
	t.Helper()
	err := s.withLock(func(d *os.File, root *os.Root) error {
		return writeReleased(d, root, map[string]ReleasedV1{reservationKey(b): {NodeID: b.NodeID, Port: b.Port, TunnelID: b.Spec.TunnelID,
			InstanceID: b.Spec.InstanceID, Generation: b.Spec.Generation, OperationID: b.OperationID, LeaseID: b.LeaseID, PlanSHA256: b.PlanSHA256, SpentJobs: spent}})
	})
	if err != nil {
		t.Fatal(err)
	}
}

// After a release only a STRICTLY greater generation with fresh operation,
// lease and plan identities may reserve the port again; the released binding
// itself (replay of the old Apply) stays refused.
func TestReservePendingAfterReleaseIsMonotonic(t *testing.T) {
	s, err := OpenReservationStore(privateReservationDir(t))
	if err != nil {
		t.Fatal(err)
	}
	old := bindingFixture(t)
	seedReleased(t, s, old, "apply-1", "retire-1")
	for name, b := range map[string]BindingV1{
		"same binding (replayed Apply)":  old,
		"same generation, fresh ids":     func() BindingV1 { n := nextGen(t, old, 1, true); return n }(),
		"greater generation, reused ids": nextGen(t, old, 2, false),
	} {
		if err := s.ReservePending(b); !errors.Is(err, ErrReserved) {
			t.Errorf("%s: accepted: %v", name, err)
		}
	}
	lower := nextGen(t, old, 5, true)
	seedReleased(t, s, lower, "apply-9")
	if err := s.ReservePending(nextGen(t, old, 3, true)); !errors.Is(err, ErrReserved) {
		t.Fatalf("lower generation accepted: %v", err)
	}
	if err := s.ReservePending(nextGen(t, lower, 6, true)); err == nil {
		// fresh ids relative to the generation-5 tombstone (op-2 reused) must fail
		t.Fatal("reused identities of the latest tombstone accepted")
	}
	fresh := nextGen(t, old, 6, true)
	fresh.OperationID, fresh.LeaseID, fresh.PlanSHA256 = "op-3", "lease-3", strings.Repeat("d", 64)
	if err := s.ReservePending(fresh); err != nil {
		t.Fatalf("a strictly greater generation with fresh identities was refused: %v", err)
	}
	left, _ := s.InspectFencedLeftovers()
	if len(left) != 1 || left[0].Binding != fresh {
		t.Fatalf("claim not recorded: %+v", left)
	}
}

// A release that crashed after its tombstone but before the claim was dropped
// is completed from the tombstone alone, and the old binding stays refused.
func TestStaleReleasedClaimIsClearedButNeverReusable(t *testing.T) {
	s, err := OpenReservationStore(privateReservationDir(t))
	if err != nil {
		t.Fatal(err)
	}
	old := bindingFixture(t)
	if err := s.ReservePending(old); err != nil {
		t.Fatal(err)
	}
	// A claim whose lifecycle entry exists is NOT stale even with a tombstone.
	seedReleased(t, s, old, "apply-1")
	if err := s.ReservePending(old); !errors.Is(err, ErrReserved) {
		t.Fatalf("same binding after a crashed release accepted: %v", err)
	}
	fresh := nextGen(t, old, 2, true)
	if err := s.ReservePending(fresh); err != nil {
		t.Fatalf("next generation after a crashed release: %v", err)
	}
	if err := s.ReservePending(old); !errors.Is(err, ErrReserved) {
		t.Fatalf("old binding accepted after the next generation: %v", err)
	}
}

func TestReleaseVerifiedRefusesWithoutAnyAuthority(t *testing.T) {
	var g *PendingGuard
	if err := g.ReleaseVerified(nil, RetirementEvidenceV1{}); err == nil {
		t.Fatal("nil guard released")
	}
	if got := releasedSpent(map[string]ReleasedV1{"k": {SpentJobs: []string{"a"}}}, "a"); !got {
		t.Fatal("spent job not recognised")
	}
	if releasedSpent(nil, "a") {
		t.Fatal("unknown job reported spent")
	}
}
