package bcc

import (
 "net/http"
 "strings"
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
 catalogVersion:=""
 var revocationSequence uint64
 if s.verifiedCatalog!=nil {
  catalogStatus="unavailable"
  v,err:=s.verifiedCatalog.Verify(now)
  if err==nil { verified=&v;catalogStatus="verified";catalogVersion=v.Version;revocationSequence=v.Revocations.Sequence }
 }
 statuses:=NodeVersionStatuses(nodes,receipts,verified,now,15*time.Minute)
 writeJSON(w,http.StatusOK,map[string]any{
  "catalog_configured":s.verifiedCatalog!=nil,
  "catalog_status":catalogStatus,
  "catalog_version":catalogVersion,
  "revocation_sequence":revocationSequence,
  "nodes":statuses,
 })
}

// ConfigureVerifiedCatalog activates the on-disk catalog only after a caller
// pins its public root key and configures durable verification state.
func (s *Server) ConfigureVerifiedCatalog(c *VerifiedReleaseCatalog) {
 s.verifiedCatalog=c
}

// nodeReleaseAccept explicitly approves one authenticated signed release.
// A durable audit intent is written before any accepted trust state changes.
func (s *Server) nodeReleaseAccept(w http.ResponseWriter,r *http.Request) {
 if r.Method!=http.MethodPost {http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return}
 if !s.admin(w,r){return}
 if s.verifiedCatalog==nil {http.Error(w,"signed catalog not configured",http.StatusServiceUnavailable);return}
 var in struct {
  Version string `json:"version"`
  Commit string `json:"commit"`
  AllowDowngrade bool `json:"allow_downgrade"`
  Confirmation string `json:"confirmation"`
 }
 if err:=decodeJSON(r,&in);err!=nil{http.Error(w,"invalid request",http.StatusBadRequest);return}
 if strings.TrimSpace(in.Version)==""||strings.TrimSpace(in.Commit)=="" {http.Error(w,"version and commit required",http.StatusBadRequest);return}
 if in.Confirmation!="ACCEPT "+in.Version+" "+in.Commit {http.Error(w,"explicit version and commit confirmation required",http.StatusBadRequest);return}
 // A downgrade is a distinct high-risk operation: require an additional
 // confirmation that names the exact release identity.
 if in.AllowDowngrade && r.Header.Get("X-BAFT-Downgrade-Confirmation")!="DOWNGRADE "+in.Version+" "+in.Commit {http.Error(w,"explicit downgrade confirmation required",http.StatusBadRequest);return}
 s.mutationMu.Lock()
 defer s.mutationMu.Unlock()
 v,err:=s.verifiedCatalog.AcceptReleaseWithIntent(s.now(),in.Version,in.Commit,in.AllowDowngrade,func(v release.Verified)error {
  return s.auditAdmin(r,"release.accept.intent",v.Version,"intent",map[string]any{
   "commit":v.Commit,"allow_downgrade":in.AllowDowngrade,"revocation_sequence":v.Revocations.Sequence,
  })
 })
 if err!=nil{http.Error(w,"release acceptance failed: "+err.Error(),http.StatusConflict);return}
 if err:=s.auditAdmin(r,"release.accept",v.Version,"success",map[string]any{"commit":v.Commit,"allow_downgrade":in.AllowDowngrade});err!=nil {
  // The pre-mutation intent remains durable for reconciliation.
  http.Error(w,"release accepted but completion audit failed; reconcile audit intent",http.StatusInternalServerError);return
 }
 writeJSON(w,http.StatusOK,map[string]any{"status":"accepted","version":v.Version,"commit":v.Commit,"revocation_sequence":v.Revocations.Sequence})
}
