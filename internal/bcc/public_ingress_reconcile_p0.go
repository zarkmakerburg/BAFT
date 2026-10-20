package bcc

import "fmt"

// PublicIngressObserved is agent-reported evidence, not BCC's desired state.
type PublicIngressObserved struct {
 Spec PublicIngressSpec
 ListenerReady bool
 FirewallReady bool
 OwnerVerified bool
}

// PublicIngressReconcileDecision is deliberately pure and never performs a
// host mutation. Only a signed agent action can apply a change.
type PublicIngressReconcileDecision string

const (
 PublicIngressNoop PublicIngressReconcileDecision = "NOOP"
 PublicIngressNeedsApply PublicIngressReconcileDecision = "NEEDS_APPLY"
 PublicIngressBlocked PublicIngressReconcileDecision = "BLOCKED"
)

// DecidePublicIngressReconcile fails closed when the agent reports an
// unowned listener or an inconsistent mapping on the requested port.
func DecidePublicIngressReconcile(desired PublicIngressSpec, observed *PublicIngressObserved) (PublicIngressReconcileDecision,error) {
 canonicalDesired,err:=PlanPublicIngress(desired.IRNode,desired.TunnelID,desired.PublicPort,desired.LocalTarget)
 if err!=nil{return PublicIngressBlocked,err}
 if observed==nil {return PublicIngressNeedsApply,nil}
 canonicalObserved,err:=PlanPublicIngress(observed.Spec.IRNode,observed.Spec.TunnelID,observed.Spec.PublicPort,observed.Spec.LocalTarget)
 if err!=nil {return PublicIngressBlocked,fmt.Errorf("invalid ingress observation: %w",err)}
 if canonicalObserved.IRNode!=canonicalDesired.IRNode || canonicalObserved.PublicPort!=canonicalDesired.PublicPort {
  return PublicIngressBlocked,fmt.Errorf("ingress observation does not match requested node and port")
 }
 if !observed.OwnerVerified {
  return PublicIngressBlocked,fmt.Errorf("ingress ownership cannot be verified")
 }
 if canonicalObserved!=canonicalDesired {
  return PublicIngressBlocked,fmt.Errorf("ingress ownership or local target differs from desired state")
 }
 if observed.ListenerReady && observed.FirewallReady {return PublicIngressNoop,nil}
 return PublicIngressNeedsApply,nil
}
