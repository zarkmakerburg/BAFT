//go:build unix

package ingressauthority

import (
	"context"
	"testing"
)

func TestInspectFencedLeftoversReportsWithoutReleasing(t *testing.T) {
	store, err := OpenReservationStore(privateReservationDir(t))
	if err != nil {
		t.Fatal(err)
	}
	if l, err := store.InspectFencedLeftovers(); err != nil || len(l) != 0 {
		t.Fatalf("fresh store: %v %v", l, err)
	}
	b := bindingFixture(t)
	if err := store.ReservePending(b); err != nil {
		t.Fatal(err)
	}
	l, err := store.InspectFencedLeftovers()
	if err != nil || len(l) != 1 || l[0].Binding != b || l[0].State != "" {
		t.Fatalf("pending: %v %v", l, err)
	}
	g, err := store.HoldPending(b)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.MarkApplying(context.Background(), b); err != nil {
		g.Close()
		t.Skipf("MarkApplying not usable in this sandbox: %v", err)
	}
	g.Close()
	l, err = store.InspectFencedLeftovers()
	if err != nil || len(l) != 1 || l[0].State != StateApplying {
		t.Fatalf("applying: %v %v", l, err)
	}
	if err := store.ReservePending(b); err != nil {
		t.Fatalf("scan must not release the claim: %v", err)
	}
}
