package ingressownership

import (
	"errors"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"strings"
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

func TestMemoryFirewallSupportsPrivileged443WithoutHostMutation(t *testing.T) {
	f := NewMemoryFirewall()
	owner := ingressmanifest.Proof{TunnelID: "low-port", Generation: 1, ConfigSHA256: strings.Repeat("a", 64), ManifestSHA256: strings.Repeat("b", 64)}
	if err := f.Install(443, owner); err != nil {
		t.Fatal(err)
	}
	ok, err := f.Verify(443, owner)
	if err != nil || !ok {
		t.Fatalf("443 verification: %v %v", ok, err)
	}
	if err := f.Remove(443, owner); err != nil {
		t.Fatal(err)
	}
	if err := f.Install(0, owner); !errors.Is(err, ErrInvalid) {
		t.Fatalf("port zero: %v", err)
	}
}
