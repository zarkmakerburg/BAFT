//go:build unix

package ingressauthority

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressinventory"
)

// Seed the durable state at the release crash boundary. These are store
// regressions, not evidence of kernel retirement or signed Agent execution.
func releaseCrashStore(t *testing.T) (*ReservationStore, BindingV1) {
	t.Helper()
	s, err := OpenReservationStore(privateReservationDir(t))
	if err != nil {
		t.Fatal(err)
	}
	b := bindingFixture(t)
	if err := s.ReservePending(b); err != nil {
		t.Fatal(err)
	}
	g, err := s.HoldPending(b)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.MarkApplying(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	return s, b
}

func seedReleaseCrashLifecycle(t *testing.T, s *ReservationStore, b BindingV1) {
	t.Helper()
	cmd := commandIntentFixture(t, b)
	err := s.withLock(func(d *os.File, root *os.Root) error {
		return writeLifecycle(d, root, map[string]lifecycleEntry{reservationKey(b): {
			Binding: b, State: StateNeedsReconciliation,
			Policy: &InstalledPolicyIdentityV1{NodeID: b.NodeID,
				ProfilePath: ingressinventory.InstalledPolicyPath, ProfileSHA256: strings.Repeat("e", 64),
				ProfileDevice: 1, ProfileInode: 1, NetworkNamespace: "net:[1]",
				SchemaVersion: 1, InventoryVersion: ingressinventory.ManagedCoverageVersion},
			Commands: map[string]CommandV1{commandIntentApply: cmd},
			Leased: map[string]string{commandIntentApply: cmd.JobID},
			Superseded: []string{"retire-spent-1"},
		}})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestReleaseCrashNeverAcknowledgesReleasedBinding(t *testing.T) {
	s, old := releaseCrashStore(t)
	seedReleased(t, s, old, "apply-intent-1")
	if err := s.ReservePending(old); !errors.Is(err, ErrReserved) {
		t.Fatalf("released binding was acknowledged as an idempotent reservation: %v", err)
	}
}

func TestReleaseCrashCompletesLifecycleBeforeReusingPort(t *testing.T) {
	s, old := releaseCrashStore(t)
	seedReleaseCrashLifecycle(t, s, old)
	seedReleased(t, s, old, "apply-intent-1", "retire-spent-1")
	// Reopen the stores, retaining only the committed on-disk evidence.
	s, err := OpenReservationStore(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	fresh := nextGen(t, old, 2, true)
	if err := s.ReservePending(fresh); err != nil {
		t.Fatalf("release stopped after tombstone could not complete: %v", err)
	}
	err = s.withLock(func(_ *os.File, root *os.Root) error {
		entries, err := readLifecycle(root)
		if err != nil {
			return err
		}
		if _, exists := entries[reservationKey(old)]; exists {
			t.Error("released lifecycle survived port reuse")
		}
		claims, err := loadPending(root)
		if err != nil {
			return err
		}
		if claims[reservationKey(fresh)] != fresh {
			t.Error("new reservation not persisted")
		}
		rel, err := loadReleased(root)
		if err == nil && (!releasedSpent(rel, "apply-intent-1") || !releasedSpent(rel, "retire-spent-1")) {
			t.Error("release recovery discarded spent jobs")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ReservePending(old); !errors.Is(err, ErrReserved) {
		t.Fatalf("released binding replay accepted: %v", err)
	}
}

func TestReleaseCrashPreservesUncoveredLifecycleJobs(t *testing.T) {
	for _, spent := range [][]string{{"apply-intent-1"}, {"retire-spent-1"}} {
		s, old := releaseCrashStore(t)
		seedReleaseCrashLifecycle(t, s, old)
		seedReleased(t, s, old, spent...)
		before, err := os.ReadFile(s.dir + "/" + lifecycleFile)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.ReservePending(nextGen(t, old, 2, true)); !errors.Is(err, ErrReserved) {
			t.Fatalf("tombstone lacking spent jobs released lifecycle: %v", err)
		}
		after, err := os.ReadFile(s.dir + "/" + lifecycleFile)
		if err != nil || string(before) != string(after) {
			t.Fatalf("unexplained lifecycle was changed: %v", err)
		}
	}
}

func TestReleaseCrashRejectsForeignTombstoneIdentity(t *testing.T) {
	for _, field := range []string{"tunnel", "instance"} {
		t.Run(field, func(t *testing.T) {
			s, old := releaseCrashStore(t)
			// This boundary follows lifecycle removal but precedes claim removal.
			if err := s.withLock(func(d *os.File, root *os.Root) error {
				return writeLifecycle(d, root, map[string]lifecycleEntry{})
			}); err != nil {
				t.Fatal(err)
			}
			foreign := old
			if field == "tunnel" {
				foreign.Spec.TunnelID = "other-tunnel"
			} else {
				foreign.Spec.InstanceID = "other-instance"
			}
			seedReleased(t, s, foreign, "apply-intent-1")
			if err := s.ReservePending(nextGen(t, old, 2, true)); !errors.Is(err, ErrReserved) {
				t.Fatalf("foreign %s tombstone cleared another claim: %v", field, err)
			}
		})
	}
}
