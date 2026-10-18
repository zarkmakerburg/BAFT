package bcc

import (
 "errors"
 "strings"
 "time"
)

// ConfirmNodePlatform records an explicit out-of-band operator attestation.
// Only the authenticated HTTP admin route may invoke this method; the agent's
// digest report cannot set this provenance. The audit intent and node document
// are committed in the same state transaction.
func (s *Store) ConfirmNodePlatform(nodeID, goos, arch string, now time.Time, audit AuditEntry) (Node, error) {
 if strings.TrimSpace(nodeID)=="" {return Node{},errors.New("node id required")}
 if now.IsZero() {return Node{},errors.New("confirmation time required")}
 if _,err:=ComponentArtifactName("agent",goos,arch);err!=nil{return Node{},err}
 if audit.Actor=="" {return Node{},errors.New("confirmation audit actor required")}
 s.mu.Lock()
 defer s.mu.Unlock()
 n,ok:=s.st.Nodes[nodeID]
 if !ok{return Node{},errors.New("node not found")}
 if n.Revoked{return Node{},errors.New("node is revoked")}
 before,err:=cloneState(s.st)
 if err!=nil{return Node{},err}
 now=now.UTC()
 n.Platform=NodePlatformEvidence{
  OS:goos,Arch:arch,Confirmed:true,Source:"operator-verified",ConfirmedAt:now,
 }
 n.UpdatedAt=now
 s.st.Nodes[nodeID]=n
 audit.Timestamp,audit.Action,audit.Target,audit.Outcome=now,"node.platform.confirm",nodeID,"success"
 if _,err:=s.enqueueSecurityAuditLocked(audit);err!=nil{s.st=before;return Node{},err}
 if err:=s.saveLocked();err!=nil{s.st=before;return Node{},err}
 return publicNode(n),nil
}
