package ingressownership

import (
	"errors"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

var ErrUnverified = errors.New("ingress restart ownership cannot be verified")

// RecoverPrepared is deliberately fail-closed: a staged manifest cannot
// establish that the process owns a live listener or firewall rule after
// restart. No socket or firewall mutation occurs during this check.
func (r *Runtime) RecoverPrepared(store *ingressmanifest.Store, expected ingressmanifest.Proof) error {
	if r == nil || store == nil {
		return ErrInvalid
	}
	inspection, err := store.Inspect(expected)
	if err != nil {
		return err
	}
	if inspection.Manifest.Phase != ingressmanifest.PhasePrepared || !inspection.ManifestBytesMatch || !inspection.PreparedBytesMatch {
		return ErrUnverified
	}
	return ErrUnverified
}
