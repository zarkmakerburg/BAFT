package bcc

import "fmt"

// ValidatePublicIngressReservation rejects a public port already reserved on
// the same IR node by a different tunnel. This is an in-memory preflight;
// host socket occupancy must still be checked by the agent before apply.
// A matching tunnel ID is idempotent only if the full specification matches.
func ValidatePublicIngressReservation(candidate PublicIngressSpec, existing []PublicIngressSpec) error {
 if _, err := PlanPublicIngress(candidate.IRNode, candidate.TunnelID, candidate.PublicPort, candidate.LocalTarget); err != nil {
  return err
 }
 for _, old := range existing {
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
