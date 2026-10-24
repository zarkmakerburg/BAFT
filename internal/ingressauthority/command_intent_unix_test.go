//go:build unix

package ingressauthority

import (
	"context"
	"testing"
	"time"
)

func commandIntentFixture(t *testing.T, b BindingV1) CommandV1 {
	t.Helper()
	now := time.Now().UTC()
	return CommandV1{IngressProtocol: ProtocolV1, JobID: "apply-intent-1", Action: "public_ingress_apply", IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Binding: b}
}

func TestCommandIntentRequiresHeldGuardAndPinnedInstalledPolicy(t *testing.T) {
	b := bindingFixture(t)
	store, err := OpenReservationStore(privateReservationDir(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReservePending(b); err != nil {
		t.Fatal(err)
	}
	g, err := store.HoldPending(b)
	if err != nil {
		t.Fatal(err)
	}
	cmd := commandIntentFixture(t, b)
	if err := g.RecordCommandIntent(context.Background(), cmd); err == nil {
		t.Fatal("command intent recorded without independently pinned installed policy")
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if err := g.RecordCommandIntent(context.Background(), cmd); err == nil {
		t.Fatal("closed OS guard recorded command intent")
	}
	if err := ((*PendingGuard)(nil)).RecordCommandIntent(context.Background(), cmd); err == nil {
		t.Fatal("nil guard recorded command intent")
	}
	intents, err := store.InspectCommandIntents(b)
	if err != nil || len(intents) != 0 {
		t.Fatalf("failed admission changed durable command data: %+v %v", intents, err)
	}
}
