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
	Journal  MutationJournal // optional durable LAB intent and phase record
}

func (t *LabTransaction) Apply(ctx context.Context, spec ingressmanifest.Spec, proof ingressmanifest.Proof) error {
	if t == nil || t.Runtime == nil || t.Firewall == nil || ctx == nil {
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
	if err != nil || n < 1 || n > 65535 {
		return ErrInvalid
	}
	record := MutationRecord{Port: uint16(n), Owner: proof, Phase: PhaseIntent}
	// A live session must not be acknowledged unless its firewall proof matches.
	t.Runtime.mu.Lock()
	existing := t.Runtime.sessions[spec.Config.Listen]
	t.Runtime.mu.Unlock()
	if existing != nil {
		if t.Journal != nil {
			current, found, err := t.Journal.Current(proof)
			if err != nil {
				return err
			}
			if !found || current.Port != record.Port || current.Owner != proof || current.Phase != PhaseVerified {
				return ErrUnverified
			}
		}
		// A replay cannot re-authorize a session with a cancelled parent,
		// even if its existing firewall ownership is still valid.
		if err := ctx.Err(); err != nil {
			return err
		}
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
	if t.Journal != nil {
		current, found, err := t.Journal.Current(proof)
		if err != nil {
			return err
		}
		if found && (current.Port != record.Port || current.Owner != proof || current.Phase != PhaseIntent) {
			return ErrUnverified
		}
		if err := t.Journal.Advance(record); err != nil {
			return err
		}
	}
	if err := t.Runtime.Apply(ctx, spec, proof); err != nil {
		return err
	}
	if t.Journal != nil {
		record.Phase = PhaseListenerBound
		if err := t.Journal.Advance(record); err != nil {
			return errors.Join(err, t.Runtime.Retire(spec.Config.Listen, proof))
		}
	}
	// A cancelled parent must never result in a success ACK after firewall setup.
	if err := ctx.Err(); err != nil {
		retireErr := t.Runtime.Retire(spec.Config.Listen, proof)
		return errors.Join(err, retireErr)
	}
	if err := t.Firewall.Install(uint16(n), proof); err != nil {
		retireErr := t.Runtime.Retire(spec.Config.Listen, proof)
		if retireErr != nil {
			return errors.Join(err, fmt.Errorf("rollback listener: %w", retireErr))
		}
		return err
	}
	if t.Journal != nil {
		record.Phase = PhaseFirewallInstalled
		if err := t.Journal.Advance(record); err != nil {
			if retireErr := t.Runtime.Retire(spec.Config.Listen, proof); retireErr != nil {
				return errors.Join(err, retireErr)
			}
			return errors.Join(err, t.Firewall.Remove(uint16(n), proof))
		}
	}
	ok, err := t.Firewall.Verify(uint16(n), proof)
	if err != nil || !ok {
		// Never remove policy before the owned listener has stopped.
		retireErr := t.Runtime.Retire(spec.Config.Listen, proof)
		if retireErr != nil {
			return errors.Join(ErrFirewallUnverified, err, retireErr)
		}
		removeErr := t.Firewall.Remove(uint16(n), proof)
		return errors.Join(ErrFirewallUnverified, err, removeErr)
	}
	if err := ctx.Err(); err != nil {
		retireErr := t.Runtime.Retire(spec.Config.Listen, proof)
		if retireErr != nil {
			return errors.Join(err, retireErr)
		}
		removeErr := t.Firewall.Remove(uint16(n), proof)
		return errors.Join(err, removeErr)
	}
	// Detect a listener that stopped while the firewall rule was installed.
	t.Runtime.mu.Lock()
	session := t.Runtime.sessions[spec.Config.Listen]
	stopped := false
	if session != nil {
		select {
		case <-session.done:
			stopped = true
		default:
		}
	}
	t.Runtime.mu.Unlock()
	if stopped {
		retireErr := t.Runtime.Retire(spec.Config.Listen, proof)
		if retireErr != nil {
			return errors.Join(ErrStopped, retireErr)
		}
		removeErr := t.Firewall.Remove(uint16(n), proof)
		return errors.Join(ErrStopped, removeErr)
	}
	// Fence a cancellation that raced with the final listener-health check.
	if err := ctx.Err(); err != nil {
		retireErr := t.Runtime.Retire(spec.Config.Listen, proof)
		if retireErr != nil {
			return errors.Join(err, retireErr)
		}
		removeErr := t.Firewall.Remove(uint16(n), proof)
		return errors.Join(err, removeErr)
	}
	if t.Journal != nil {
		record.Phase = PhaseVerified
		if err := t.Journal.Advance(record); err != nil {
			if retireErr := t.Runtime.Retire(spec.Config.Listen, proof); retireErr != nil {
				return errors.Join(err, retireErr)
			}
			return errors.Join(err, t.Firewall.Remove(uint16(n), proof))
		}
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
	if err != nil || n < 1 || n > 65535 {
		return ErrInvalid
	}
	if t.Journal != nil {
		current, found, err := t.Journal.Current(proof)
		if err != nil {
			return err
		}
		if !found || current.Port != uint16(n) || current.Owner != proof || current.Phase != PhaseVerified {
			return ErrUnverified
		}
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
	if err := t.Firewall.Remove(uint16(n), proof); err != nil {
		return err
	}
	if t.Journal != nil {
		return t.Journal.Advance(MutationRecord{Port: uint16(n), Owner: proof, Phase: PhaseRetired})
	}
	return nil
}
