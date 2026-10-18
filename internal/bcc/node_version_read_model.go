package bcc

import (
 "sort"
 "time"

 "github.com/zarkmakerburg/baft/internal/release"
)

// NodeComponentStatus is the read-only BCC representation of a component
// digest. A version is exposed only after signed release verification and
// operator-confirmed target architecture. Unconfigured catalogs stay unknown.
type NodeComponentStatus struct {
 Component string `json:"component"`
 State ComponentTrustState `json:"state"`
 Version string `json:"version,omitempty"`
 ObservedAt time.Time `json:"observed_at,omitempty"`
}

type NodeVersionStatus struct {
 NodeID string `json:"node_id"`
 Platform NodePlatformEvidence `json:"platform"`
 Components []NodeComponentStatus `json:"components"`
}

// NodeComponentReadModel fails closed on missing/revoked nodes, unknown or
// stale reports, or untrusted platform evidence. The caller supplies only a
// Verified value produced by release.VerifyDir against the pinned root.
func NodeComponentReadModel(node Node, receipt ComponentDigestReceipt, verified *release.Verified, now time.Time, maxAge time.Duration) NodeVersionStatus {
 result:=NodeVersionStatus{NodeID:node.ID,Platform:node.Platform}
 components:=[]string{"transport","agent","bcc"}
 if now.IsZero(){now=time.Now().UTC()}
 for _,component:=range components {
  item:=NodeComponentStatus{Component:component,State:ComponentUnknown}
  // Deliberately don't expose a version for a stale/missing receipt. Agent
  // timestamps alone cannot authenticate freshness: use server AcceptedAt.
  if !node.Revoked && node.Platform.Confirmed && node.Platform.Source=="operator-verified" &&
    verified!=nil && receipt.NodeID==node.ID &&
    !receipt.AcceptedAt.IsZero() && !receipt.AcceptedAt.After(now) &&
    maxAge>0 && now.Sub(receipt.AcceptedAt)<=maxAge &&
    !node.Platform.ConfirmedAt.IsZero() && !receipt.AcceptedAt.Before(node.Platform.ConfirmedAt) {
   artifact,ok:=TrustedNodeArtifactName(component,node.Platform)
   if ok {
    state:=CompareVerifiedComponentDigest(receipt.Digests[component],*verified,artifact)
    item.State=state
    if state==ComponentVerified {item.Version=verified.Version}
   }
  }
  if receipt.NodeID==node.ID {item.ObservedAt=receipt.ObservedAt}
  result.Components=append(result.Components,item)
 }
 return result
}

// NodeVersionStatuses computes stable ordering without ever consulting agent
// claimed versions. Missing trusted manifest means all components are unknown.
func NodeVersionStatuses(nodes []Node, receipts map[string]ComponentDigestReceipt, verified *release.Verified, now time.Time, maxAge time.Duration) []NodeVersionStatus {
 out:=make([]NodeVersionStatus,0,len(nodes))
 for _,node:=range nodes {out=append(out,NodeComponentReadModel(node,receipts[node.ID],verified,now,maxAge))}
 sort.Slice(out,func(i,j int)bool{return out[i].NodeID<out[j].NodeID})
 return out
}
