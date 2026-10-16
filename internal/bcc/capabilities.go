package bcc

import (
	"fmt"
	"time"

	"github.com/zarkmakerburg/baft/internal/verification"
)

// Capability reports come from an authenticated agent poll. Unknown/older
// clients revoke the previous claim instead of inheriting another binary's
// capability after a downgrade. Dispatch remains a separate durable operation.
func (s *Store) PullJobsWithCapabilities(node, token, capability string) ([]Job, error) {
	s.mu.Lock()
	if !s.authorizedLocked(node, token) {
		s.mu.Unlock()
		return nil, ErrAgentAuthentication
	}
	previous := s.st.Nodes[node]
	n := previous
	now := time.Now().UTC()
	version := 0
	if capability == verification.Capability {
		version = verification.Version
	}
	n.TunnelVerification, n.CapabilitiesAt, n.AgentSeen = version, now, now
	s.st.Nodes[node] = n
	if previous.TunnelVerification != version || previous.CapabilitiesAt.IsZero() || now.Sub(previous.CapabilitiesAt) >= time.Minute {
		if err := s.saveLocked(); err != nil {
			s.st.Nodes[node] = previous
			s.mu.Unlock()
			return nil, err
		}
	}
	s.mu.Unlock()
	return s.PullJobs(node, token)
}

func supportsTunnelVerification(n Node, now time.Time) bool {
	return n.TunnelVerification == verification.Version && !n.CapabilitiesAt.IsZero() && !n.CapabilitiesAt.After(now) && now.Sub(n.CapabilitiesAt) < agentFreshness
}

func requireTunnelVerification(ex, ir Node, now time.Time) error {
	for _, n := range []Node{ex, ir} {
		if !supportsTunnelVerification(n, now) {
			return fmt.Errorf("node %s has no fresh verified data-delivery capability; update BAFT and its agent before deployment", n.ID)
		}
	}
	return nil
}
