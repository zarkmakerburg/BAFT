package ingressownership

import (
	"errors"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"sync"
)

// Firewall is an isolated policy contract. A production implementation must
// verify kernel rule identity before returning success; never use port alone
// as proof of ownership. No shell commands are executed by this package.
type Firewall interface {
	Install(port uint16, owner ingressmanifest.Proof) error
	Remove(port uint16, owner ingressmanifest.Proof) error
	Verify(port uint16, owner ingressmanifest.Proof) (bool, error)
}

// MemoryFirewall is a test double, NOT a host firewall implementation.
type MemoryFirewall struct {
	mu    sync.Mutex
	rules map[uint16]ingressmanifest.Proof
}

func NewMemoryFirewall() *MemoryFirewall {
	return &MemoryFirewall{rules: make(map[uint16]ingressmanifest.Proof)}
}
func (f *MemoryFirewall) Install(port uint16, owner ingressmanifest.Proof) error {
	if f == nil || port == 0 || !valid(owner) {
		return ErrInvalid
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if old, ok := f.rules[port]; ok && old != owner {
		return ErrConflict
	}
	f.rules[port] = owner
	return nil
}
func (f *MemoryFirewall) Remove(port uint16, owner ingressmanifest.Proof) error {
	if f == nil || port == 0 || !valid(owner) {
		return ErrInvalid
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.rules[port] != owner {
		return ErrUnknown
	}
	delete(f.rules, port)
	return nil
}
func (f *MemoryFirewall) Verify(port uint16, owner ingressmanifest.Proof) (bool, error) {
	if f == nil || port == 0 || !valid(owner) {
		return false, ErrInvalid
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	current, ok := f.rules[port]
	return ok && current == owner, nil
}

var ErrFirewallUnverified = errors.New("firewall rule owner could not be verified")
