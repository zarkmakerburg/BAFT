package bcc

import "fmt"

// ValidatePublicIngressReservation checks desired-state ownership only.
// Agent-side listener and firewall verification remain separate gates.
// Both candidate and stored reservations must be canonicalized before
// comparing ownership to prevent whitespace and loopback alias bypasses.
func ValidatePublicIngressReservation(candidate PublicIngressSpec, existing []PublicIngressSpec) error {
	candidate, err := PlanPublicIngress(candidate.IRNode, candidate.TunnelID, candidate.PublicPort, candidate.LocalTarget)
	if err != nil {
		return err
	}

	// Validate the complete stored set before making any ownership decision.
	// Invalid records on other nodes are still unsafe to silently ignore.
	canonical := make([]PublicIngressSpec, 0, len(existing))
	for _, raw := range existing {
		old, err := PlanPublicIngress(raw.IRNode, raw.TunnelID, raw.PublicPort, raw.LocalTarget)
		if err != nil {
			return fmt.Errorf("invalid existing public ingress reservation: %w", err)
		}
		canonical = append(canonical, old)
	}
	// Check the entire stored set in linear time before admitting a candidate.
	// Canonical duplicates are harmless; conflicting ownership is not.
	type nodeTunnel struct{ node, tunnel string }
	type nodePort struct{ node string; port int }
	byTunnel := make(map[nodeTunnel]PublicIngressSpec, len(canonical))
	byPort := make(map[nodePort]PublicIngressSpec, len(canonical))
	for _, old := range canonical {
		tk := nodeTunnel{old.IRNode, old.TunnelID}
		if prior, ok := byTunnel[tk]; ok {
			if prior.PublicPort != old.PublicPort || prior.LocalTarget != old.LocalTarget {
				return fmt.Errorf("conflicting stored public ingress reservations for tunnel %s", old.TunnelID)
			}
		} else {
			byTunnel[tk] = old
		}
		pk := nodePort{old.IRNode, old.PublicPort}
		if prior, ok := byPort[pk]; ok {
			if prior.TunnelID != old.TunnelID {
				return fmt.Errorf("conflicting stored public ingress reservations on IR node %s port %d", old.IRNode, old.PublicPort)
			}
		} else {
			byPort[pk] = old
		}
	}
	for _, old := range canonical {
		if old.IRNode != candidate.IRNode {
			continue
		}
		if old.TunnelID == candidate.TunnelID {
			if old.PublicPort != candidate.PublicPort || old.LocalTarget != candidate.LocalTarget {
				return fmt.Errorf("tunnel %s already has a different public ingress reservation", candidate.TunnelID)
			}
			continue
		}
		if old.PublicPort == candidate.PublicPort {
			return fmt.Errorf("IR node %s public port %d is reserved by tunnel %s", candidate.IRNode, candidate.PublicPort, old.TunnelID)
		}
	}
	return nil
}
