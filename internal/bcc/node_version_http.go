package bcc

import (
 "net/http"
 "time"
 "github.com/zarkmakerburg/baft/internal/release"
)

// nodeVersions exposes authenticated observations only. Any missing or invalid
// signed catalog evidence fails closed to unknown component states.
func (s *Server) nodeVersions(w http.ResponseWriter,r *http.Request) {
 if r.Method!=http.MethodGet {http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return}
 if !s.admin(w,r){return}
 now:=s.now()
 nodes,receipts:=s.store.ComponentVersionSnapshot()
 var verified *release.Verified
 catalogStatus:="not_configured"
 if s.verifiedCatalog!=nil {
  catalogStatus="unavailable"
  v,err:=s.verifiedCatalog.Verify(now)
  if err==nil { verified=&v;catalogStatus="verified" }
 }
 statuses:=NodeVersionStatuses(nodes,receipts,verified,now,15*time.Minute)
 writeJSON(w,http.StatusOK,map[string]any{
  "catalog_configured":s.verifiedCatalog!=nil,
  "catalog_status":catalogStatus,
  "nodes":statuses,
 })
}

// ConfigureVerifiedCatalog activates the on-disk catalog only after a caller
// pins its public root key and configures durable verification state.
func (s *Server) ConfigureVerifiedCatalog(c *VerifiedReleaseCatalog) {
 s.verifiedCatalog=c
}
