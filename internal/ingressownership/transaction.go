package ingressownership

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// LabTransaction coordinates a test firewall and the isolated TCP runtime.
// A real host firewall is deliberately NOT supported by this entry point.
type LabTransaction struct {
	mu       sync.Mutex
	Runtime  *Runtime
	Firewall *MemoryFirewall
}

func (t *LabTransaction) Apply(ctx context.Context, spec ingressmanifest.Spec, proof ingressmanifest.Proof) error {
	if t == nil || t.Runtime == nil || t.Firewall == nil {
		return ErrInvalid
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	expected, err := ingressmanifest.Preview(spec)
	if err != nil {
		return err
	}
	if expected != proof {
		return ErrInvalid
	}
	canonical, err := canonicalEndpoint(spec.Config.Listen)
	if err != nil {
		return err
	}
	spec.Config.Listen = canonical
	_, p, err := net.SplitHostPort(spec.Config.Listen)
	if err != nil {
		return err
	}
	n, err := strconv.Atoi(p)
	if err != nil || n < 1024 || n > 65535 {
		return ErrInvalid
	}
	// A live session must not be acknowledged unless its firewall proof matches.
	t.Runtime.mu.Lock()
	existing := t.Runtime.sessions[spec.Config.Listen]
	t.Runtime.mu.Unlock()
	if existing != nil {
		if existing.proof != proof {
			return ErrConflict
		}
		ok, err := t.Firewall.Verify(uint16(n), proof)
		if err != nil {
			return err
		}
		if !ok {
			return ErrFirewallUnverified
		}
		return t.Runtime.Apply(ctx, spec, proof)
	}
	if err := t.Runtime.Apply(ctx, spec, proof); err != nil {
		return err
	}
	if err := t.Firewall.Install(uint16(n), proof); err != nil {
		retireErr := t.Runtime.Retire(spec.Config.Listen, proof)
		if retireErr != nil {
			return errors.Join(err, fmt.Errorf("rollback listener: %w", retireErr))
		}
		return err
	}
	ok, err := t.Firewall.Verify(uint16(n), proof)
	if err != nil || !ok {
		removeErr := t.Firewall.Remove(uint16(n), proof)
		retireErr := t.Runtime.Retire(spec.Config.Listen, proof)
		return errors.Join(ErrFirewallUnverified, err, removeErr, retireErr)
	}
	return nil
}
func (t *LabTransaction) Retire(endpoint string, proof ingressmanifest.Proof) error {
	if t == nil || t.Runtime == nil || t.Firewall == nil {
		return ErrInvalid
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	canonical, err := canonicalEndpoint(endpoint)
	if err != nil {
		return err
	}
	endpoint = canonical
	_, p, err := net.SplitHostPort(endpoint)
	if err != nil {
		return err
	}
	n, err := strconv.Atoi(p)
	if err != nil || n < 1024 || n > 65535 {
		return ErrInvalid
	}
	ok, err := t.Firewall.Verify(uint16(n), proof)
	if err != nil {
		return err
	}
	if !ok {
		return ErrFirewallUnverified
	}
	// Fail closed: never remove a firewall rule until exact-owner socket retire succeeds.
	if err := t.Runtime.Retire(endpoint, proof); err != nil {
		return err
	}
	return t.Firewall.Remove(uint16(n), proof)
}
