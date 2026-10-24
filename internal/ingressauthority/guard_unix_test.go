//go:build unix

package ingressauthority

import (
	"errors"
	"testing"
)

func TestPendingGuard(t *testing.T) {
	dir := privateReservationDir(t)
	s, err := OpenReservationStore(dir)
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
	if other, err := s.HoldPending(b); !errors.Is(err, ErrGuardBusy) {
		if other != nil {
			other.Close()
		}
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.ReservePending(b); err != nil {
		t.Fatal(err)
	}
}
