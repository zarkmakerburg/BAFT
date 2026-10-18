package bcc

import (
 "net/http"
 "time"
)

// nodeVersions exposes only authenticated observations with conservative
// unknown states. Signed release catalog verification is deliberately not
// enabled until pinned-root and durable anti-rollback state are wired.
func (s *Server) nodeVersions(w http.ResponseWriter,r *http.Request) {
 if r.Method!=http.MethodGet {http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return}
 if !s.admin(w,r){return}
 nodes,receipts:=s.store.ComponentVersionSnapshot()
 statuses:=NodeVersionStatuses(nodes,receipts,nil,s.now(),15*time.Minute)
 writeJSON(w,http.StatusOK,map[string]any{
  "catalog_configured":false,
  "nodes":statuses,
 })
}
