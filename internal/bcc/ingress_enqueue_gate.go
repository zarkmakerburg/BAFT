package bcc

import (
	"errors"
	"sync/atomic"
)

// ErrIngressAuthorityUnavailable is the deliberate fail-closed operational gate.
// A staged command is NOT a held live lease or an executable agent job.
var ErrIngressAuthorityUnavailable = errors.New("live ingress authority and managed executor readiness unavailable")

// ingressLiveAuthorityProbe reports whether the deployed Agent fleet has live
// held-lease authority. The probe is unexported and has no exported setter, so
// no production caller (or configuration bool) can open the gate: enabling it
// requires a deliberate code change inside this package once Agent authority
// exists. The zero state is closed.
type ingressLiveAuthorityProbe interface {
	LiveIngressAuthority() error
}

type ingressProbeBox struct{ probe ingressLiveAuthorityProbe }

var ingressAuthorityProbe atomic.Pointer[ingressProbeBox]

// ingressEnqueueGate returns nil only when a probe is installed and reports
// live authority. Every enqueue path must call it before touching state.
func ingressEnqueueGate() error {
	box := ingressAuthorityProbe.Load()
	if box == nil || box.probe == nil {
		return ErrIngressAuthorityUnavailable
	}
	if err := box.probe.LiveIngressAuthority(); err != nil {
		return errors.Join(ErrIngressAuthorityUnavailable, err)
	}
	return nil
}
