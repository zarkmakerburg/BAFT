package bcc

import (
 "net/http"
)

// nodePlatform requires an administrator to attest that the target host's
// OS/architecture was verified outside the unauthenticated agent inventory.
// It does not trust fields received from agent component reports.
func (s *Server) nodePlatform(w http.ResponseWriter,r *http.Request) {
 if r.Method!=http.MethodPost {http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return}
 if !s.admin(w,r){return}
 s.mutationMu.Lock()
 defer s.mutationMu.Unlock()
 var in struct {
  NodeID string `json:"node_id"`
  OS string `json:"os"`
  Arch string `json:"arch"`
  Confirmation string `json:"confirmation"`
 }
 if err:=decodeJSON(r,&in);err!=nil{http.Error(w,err.Error(),http.StatusBadRequest);return}
 // The caller must explicitly acknowledge checking the target host's
 // architecture by an independent channel (SSH/console, not agent digests).
 if in.Confirmation!="verified-out-of-band" {
  http.Error(w,"explicit out-of-band verification acknowledgement required",http.StatusBadRequest)
  return
 }
 details:=map[string]any{"os":in.OS,"arch":in.Arch,"confirmation":"verified-out-of-band"}
 n,err:=s.store.ConfirmNodePlatform(in.NodeID,in.OS,in.Arch,s.now(),AuditEntry{
  Timestamp:s.now().UTC(),Actor:"admin",RemoteIP:s.clientIP(r),Details:withRequest(r,details),
 })
 if err!=nil {
  s.auditFailure(w,r,"node.platform.confirm",in.NodeID,details,err,http.StatusBadRequest)
  return
 }
 if err:=s.FlushSecurityAuditIntents();err!=nil {w.Header().Set("X-BAFT-Audit-State","pending")}
 writeJSON(w,http.StatusOK,n)
}
