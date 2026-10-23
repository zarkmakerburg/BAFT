package ingressownership

import (
	"context"
	"errors"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// A previously successful Apply must not ACK an idempotent replay after its
// original context was canceled, even if Serve has not yet published done.
func TestRuntimeCancelledOwnerReplayNeverAcknowledgesHealthy(t *testing.T) {
	for iteration := 0; iteration < 200; iteration++ {
		r := NewRuntime()
		spec := testSpec(t, "cancelled-replay")
		proof, err := ingressmanifest.Preview(spec)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		if err = r.Apply(ctx, spec, proof); err != nil {
			cancel()
			t.Fatalf("initial apply: %v", err)
		}
		cancel()
		// An exact-owner replay with a fresh context MUST NOT be accepted
		// while its first owner's context is already canceled.
		err = r.Apply(context.Background(), spec, proof)
		if !errors.Is(err, ErrStopped) {
			t.Fatalf("iteration %d acknowledged cancelled listener as healthy: %v", iteration, err)
		}
		if err := r.Retire(spec.Config.Listen, proof); err != nil {
			t.Fatalf("iteration %d retiring canceled owner: %v", iteration, err)
		}
	}
}
