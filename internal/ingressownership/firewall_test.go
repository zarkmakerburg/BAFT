package ingressownership

import (
	"errors"
	"testing"
)

func TestMemoryFirewallExactOwnership(t *testing.T) {
	f := NewMemoryFirewall()
	a := proof("a", 1)
	b := proof("b", 1)
	if err := f.Install(2053, a); err != nil {
		t.Fatal(err)
	}
	if err := f.Install(2053, b); !errors.Is(err, ErrConflict) {
		t.Fatalf("foreign install: %v", err)
	}
	if err := f.Remove(2053, b); !errors.Is(err, ErrUnknown) {
		t.Fatalf("foreign remove: %v", err)
	}
	ok, err := f.Verify(2053, a)
	if err != nil || !ok {
		t.Fatalf("owner lost: %v %v", ok, err)
	}
	if err := f.Remove(2053, a); err != nil {
		t.Fatal(err)
	}
	ok, err = f.Verify(2053, a)
	if err != nil || ok {
		t.Fatalf("rule leaked: %v %v", ok, err)
	}
}
