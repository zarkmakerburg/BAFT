package bcc

import "fmt"

// ValidatePublicIngressReservation checks desired-state ownership only.
// Agent-side listener and firewall verification are separate mandatory gates.
// Canonicalization is required for both candidate and stored reservations so
// whitespace or equivalent loopback forms cannot bypass collision checks.
func ValidatePublicIngressReservation(candidate PublicIngressSpec, existing []PublicIngressSpec) error {
 candidate, err := PlanPublicIngress(candidate.IRNode, candidate.TunnelID, candidate.PublicPort, candidate.LocalTarget)
 if err != nil { return err }
 for _, raw := range existing {
  old, err := PlanPublicIngress(raw.IRNode, raw.TunnelID, raw.PublicPort, raw.LocalTarget)
  if err != nil { return fmt.Errorf("invalid existing public ingress reservation: %w", err) }
  if old.IRNode != candidate.IRNode { continue }
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
