//go:build unix

package ingressauthority

import (
	"context"
	"errors"
	"testing"
)

func TestRestartAuthorityWithLockedReconciliationRejectsInvalidAuthority(t *testing.T) {
	ran := false
	effect := func() error { ran = true; return nil }
	for name, authority := range map[string]*RestartAuthority{
		"nil": nil, "zero": {}, "missing guard": {minted: true},
		"closed guard": {minted: true, guard: &PendingGuard{}},
	} {
		if err := authority.WithLockedReconciliation(context.Background(), effect); !errors.Is(err, ErrUnverified) {
			t.Fatalf("%s authority: %v", name, err)
		}
	}
	if ran {
		t.Fatal("effect ran without restart authority")
	}
}
