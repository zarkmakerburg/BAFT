//go:build unix

package ingressauthority

import (
	"context"
	"testing"
)

func TestHeldCommandDeniesWithoutPinnedInstalledPolicy(t *testing.T) {
	store, err := OpenReservationStore(privateReservationDir(t))
	if err != nil {
		t.Fatal(err)
	}
	b := bindingFixture(t)
	if err := store.ReservePending(b); err != nil {
		t.Fatal(err)
	}
	g, err := store.HoldPending(b)
	if err != nil {
		t.Fatal(err)
	}
	command := commandIntentFixture(t, b)
	if err := g.CheckHeldCommand(context.Background(), command); err == nil {
		t.Fatal("unprovisioned command authorized")
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if err := g.CheckHeldCommand(context.Background(), command); err == nil {
		t.Fatal("closed lock authorized command")
	}
	if err := ((*PendingGuard)(nil)).CheckHeldCommand(context.Background(), command); err == nil {
		t.Fatal("nil lock authorized command")
	}
}
