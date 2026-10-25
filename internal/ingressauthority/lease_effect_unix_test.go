//go:build unix

package ingressauthority

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWithLockedIngressNeverRunsEffectWithoutLiveLease(t *testing.T) {
	ctx := context.Background()
	b := bindingFixture(t)
	now := time.Now().UTC()
	cmd := CommandV1{IngressProtocol: ProtocolV1, JobID: "apply-effect-1", Action: "public_ingress_apply",
		IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Binding: b}
	ran := false
	effect := func() error { ran = true; return nil }
	closed := &LiveLease{minted: true, binding: b, authority: &heldLease{guard: &PendingGuard{binding: b}, command: cmd}}
	for name, lease := range map[string]*LiveLease{"nil": nil, "zero": {}, "closed guard": closed} {
		if err := lease.WithLockedIngress(ctx, cmd, effect); !errors.Is(err, ErrUnverified) {
			t.Fatalf("%s lease: %v", name, err)
		}
	}
	if err := closed.WithLockedIngress(ctx, cmd, nil); !errors.Is(err, ErrUnverified) {
		t.Fatalf("nil effect: %v", err)
	}
	if ran {
		t.Fatal("effect ran without a live lease")
	}
}
