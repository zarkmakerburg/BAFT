package bcc

import (
 "net/http"
 "time"
 "github.com/zarkmakerburg/baft/internal/release"
)

// nodeVersions exposes only authenticated observations with conservative
// unknown states. Signed release catalog verification is deliberately not
// enabled until pinned-root and durable anti-rollback state are wired.
func (s *Server) nodeVersions(w http.ResponseWriter,r *http.Request) {
 if r.Method!=http.MethodGet {http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return}
 if !s.admin(w,r){return}
 nodes,receipts:=s.store.ComponentVersionSnapshot()
 var verified *release.Verified
 if s.verifiedCatalog!=nil {
  v,err:=s.verifiedCatalog.Verify(s.now())
  if err==nil { verified=&v }
 }
 statuses:=NodeVersionStatuses(nodes,receipts,verified,s.now(),15*time.Minute)
 writeJSON(w,http.StatusOK,map[string]any{
  "catalog_configured":s.verifiedCatalog!=nil,
  "nodes":statuses,
 })
}

// ConfigureVerifiedCatalog activates the on-disk catalog only after a caller
// pins its public root key and configures durable verification state.
func (s *Server) ConfigureVerifiedCatalog(c *VerifiedReleaseCatalog) {
 s.verifiedCatalog=c
}
