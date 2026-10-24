package bcc

import (
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"time"
)

// IngressCapabilityOperationalV1 is the capability an Agent declares on its
// authenticated poll when it can execute the typed operational Apply/Retire
// jobs with a held lease and managed executor.
const IngressCapabilityOperationalV1 = "operational-ingress-v1"

// ingressCapabilityMaxAge bounds how old the authenticated poll carrying the
// capability may be; a silent or restored node is therefore closed.
const ingressCapabilityMaxAge = 2 * time.Minute

// agentCapabilitiesHeader carries the comma-separated capability tokens on the
// authenticated GET /api/agent/jobs poll. Absent header == no capabilities.
const agentCapabilitiesHeader = "X-BAFT-Agent-Capabilities"

// ingressEnqueueGateLocked is the production gate for one reviewed tunnel. It
// opens only from state BCC itself verified: the IR node is enrolled, not
// revoked, and its authenticated agent poll (not older than
// ingressCapabilityMaxAge) declared operational-ingress-v1. There is no flag,
// setter or environment variable; an old agent never declares it. A test-only
// probe, when installed in-package, still takes precedence as before.
func (s *Store) ingressEnqueueGateLocked(tunnelID string, now time.Time) error {
	if box := ingressAuthorityProbe.Load(); box != nil && box.probe != nil {
		return ingressEnqueueGate()
	}
	op, ok := s.st.PublicIngressOperations[tunnelID]
	if !ok {
		return ErrIngressAuthorityUnavailable
	}
	n, ok := s.st.Nodes[op.Spec.IRNode]
	if !ok || n.Revoked || n.AgentTokenHash == "" || (n.Role != "worker" && n.Role != "master") {
		return errors.Join(ErrIngressAuthorityUnavailable, errors.New("IR node not enrolled"))
	}
	if n.AgentSeen.IsZero() || n.AgentSeen.After(now.Add(time.Minute)) || now.Sub(n.AgentSeen) > ingressCapabilityMaxAge {
		return errors.Join(ErrIngressAuthorityUnavailable, errors.New("IR node agent heartbeat is not fresh"))
	}
	if !slices.Contains(n.Capabilities, IngressCapabilityOperationalV1) {
		return errors.Join(ErrIngressAuthorityUnavailable, errors.New("IR node agent does not report "+IngressCapabilityOperationalV1))
	}
	return nil
}

// parseAgentCapabilities strictly validates the capability header values: at
// most one header, at most 8 distinct tokens of [a-z0-9-] up to 64 bytes,
// comma separated without spaces. Absent means no capabilities.
func parseAgentCapabilities(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	if len(values) != 1 {
		return nil, errors.New("repeated capability header")
	}
	parts := strings.Split(values[0], ",")
	if len(parts) > 8 {
		return nil, errors.New("too many capabilities")
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" || len(p) > 64 {
			return nil, errors.New("invalid capability token")
		}
		for i := 0; i < len(p); i++ {
			c := p[i]
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return nil, errors.New("invalid capability token")
			}
		}
		if slices.Contains(out, p) {
			return nil, errors.New("duplicate capability")
		}
		out = append(out, p)
	}
	return out, nil
}

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
