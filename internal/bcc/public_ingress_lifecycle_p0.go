package bcc

import "fmt"

// PublicIngressLifecycle is an in-memory state transition contract. It does
// not claim that an agent has created a listener or modified firewall rules.
type PublicIngressLifecycle struct {
 Spec PublicIngressSpec `json:"spec"`
 Status string `json:"status"`
 Applied bool `json:"applied"`
}

const (
 PublicIngressPending = "PENDING"
 PublicIngressApplied = "APPLIED"
 PublicIngressFailed = "FAILED"
 PublicIngressRemoved = "REMOVED"
)

// TransitionPublicIngress requires an exact ownership match for any status
// change. APPLIED is only for a caller that has independently verified the
// listener; this helper does not perform verification.
func TransitionPublicIngress(current PublicIngressLifecycle, owner PublicIngressSpec, next string) (PublicIngressLifecycle,error) {
 if current.Spec != owner { return current, fmt.Errorf("public ingress ownership mismatch") }
 valid := false
 switch current.Status {
 case PublicIngressPending:
  valid = next==PublicIngressApplied || next==PublicIngressFailed || next==PublicIngressRemoved
 case PublicIngressApplied:
  valid = next==PublicIngressFailed || next==PublicIngressRemoved
 case PublicIngressFailed:
  valid = next==PublicIngressPending || next==PublicIngressRemoved
 }
 if !valid {return current, fmt.Errorf("invalid public ingress transition %s -> %s",current.Status,next)}
 current.Status=next
 current.Applied=next==PublicIngressApplied
 return current,nil
}
