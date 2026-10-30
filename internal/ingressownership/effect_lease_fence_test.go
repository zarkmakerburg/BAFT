package ingressownership

import (
	"context"
	"errors"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
)

func TestEffectLeaseFenceRunsEffectOnlyUnderLiveLease(t *testing.T) {
	ctx := context.Background()
	for name, f := range map[string]*effectLeaseFence{
		"no lease":   {},
		"zero lease": {lease: &ingressauthority.Lease{}},
	} {
		ran := false
		err := f.atEffect(ctx, func(context.Context) error { return nil }, func() error { ran = true; return nil })
		if err == nil || ran {
			t.Fatalf("%s: effect ran=%v err=%v", name, ran, err)
		}
	}
	denied := errors.New("socket evidence changed")
	ran := false
	f := &effectLeaseFence{
		authority: func(context.Context) error { return nil },
		installed: func(context.Context) error { return nil },
	}
	if err := f.atEffect(ctx, func(context.Context) error { return denied }, func() error { ran = true; return nil }); !errors.Is(err, denied) || ran {
		t.Fatalf("pre-effect evidence denial: ran=%v err=%v", ran, err)
	}
}
