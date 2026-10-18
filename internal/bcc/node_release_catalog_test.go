package bcc

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "github.com/zarkmakerburg/baft/internal/release"
)

func TestVerifiedReleaseCatalogPinsRootAndPersistsState(t *testing.T) {
 base:=t.TempDir()
 dir:=filepath.Join(base,"signed")
 if err:=os.Mkdir(dir,0700);err!=nil{t.Fatal(err)}
 now:=time.Date(2026,10,18,16,0,0,0,time.UTC)
 rootPub,rootPriv,err:=release.GenerateKey();if err!=nil{t.Fatal(err)}
 signPub,signPriv,err:=release.GenerateKey();if err!=nil{t.Fatal(err)}
 cert,err:=release.Certify(rootPriv,signPub,now.Add(-time.Hour),now.Add(24*time.Hour));if err!=nil{t.Fatal(err)}
 rev,err:=release.SignRevocations(rootPriv,2,nil,now.Add(-time.Hour),now.Add(24*time.Hour));if err!=nil{t.Fatal(err)}
 rootPath:=filepath.Join(base,"root.pub")
 revPath:=filepath.Join(base,"revocations.json")
 trustPath:=filepath.Join(base,"trust.json")
 if err:=os.WriteFile(rootPath,[]byte(release.EncodePublic(rootPub)),0600);err!=nil{t.Fatal(err)}
 if err:=release.WriteEnvelope(revPath,rev);err!=nil{t.Fatal(err)}
 binary:=[]byte("signed BAFT transport")
 if err:=os.WriteFile(filepath.Join(dir,"baft-linux-amd64"),binary,0700);err!=nil{t.Fatal(err)}
 _,err=release.SignDir(release.SignInput{Dir:dir,Version:"v1.2.3",Commit:strings.Repeat("a",40),CreatedAt:now,Provenance:release.Provenance{Builder:"test",GoVersion:"go1.27"},Key:signPriv,Cert:cert,Root:rootPub})
 if err!=nil{t.Fatal(err)}
 cat,err:=NewVerifiedReleaseCatalog(dir,rootPath,revPath,trustPath);if err!=nil{t.Fatal(err)}
 got,err:=cat.Verify(now)
 if err!=nil||got.Version!="v1.2.3"{t.Fatalf("signed catalog rejected: %v %+v",err,got)}
 state,err:=release.ReadState(trustPath);if err!=nil{t.Fatal(err)}
 if state.Version!=""||state.RevocationSequence!=2{t.Fatalf("state not persisted %+v",state)}
 digest:=sha256.Sum256(binary)
 store,err:=OpenStore(filepath.Join(base,"bcc.sqlite"))
 if err!=nil{t.Fatal(err)}
 if _,err:=store.UpsertNode(Node{ID:"ex-1",Address:"127.0.0.1:2443",Role:"foreign"},"agent-key");err!=nil{t.Fatal(err)}
 if _,err:=store.ConfirmNodePlatform("ex-1","linux","amd64",now.Add(-time.Minute),AuditEntry{Actor:"admin"});err!=nil{t.Fatal(err)}
 store.mu.Lock()
 store.st.ComponentDigestReceipts=map[string]ComponentDigestReceipt{"ex-1":{
  NodeID:"ex-1",ObservedAt:now,AcceptedAt:now,Digests:map[string]string{"transport":hex.EncodeToString(digest[:])},
 }}
 if err:=store.saveLocked();err!=nil{store.mu.Unlock();t.Fatal(err)}
 store.mu.Unlock()
 app,err:=NewServer(store,"admin-secret")
 if err!=nil{t.Fatal(err)}
 app.now=func()time.Time{return now}
 app.ConfigureVerifiedCatalog(cat)
 response:=httptest.NewRecorder()
 app.Handler().ServeHTTP(response,authReq(http.MethodGet,"/api/nodes/versions","admin-secret",nil))
 if response.Code!=http.StatusOK{t.Fatalf("status=%d",response.Code)}
 var model struct {
  CatalogConfigured bool `json:"catalog_configured"`
  CatalogStatus string `json:"catalog_status"`
  CatalogVersion string `json:"catalog_version"`
  RevocationSequence uint64 `json:"revocation_sequence"`
  Nodes []NodeVersionStatus `json:"nodes"`
 }
 if err:=json.Unmarshal(response.Body.Bytes(),&model);err!=nil{t.Fatal(err)}
 if !model.CatalogConfigured||model.CatalogStatus!="verified"||model.CatalogVersion!="v1.2.3"||model.RevocationSequence!=2||len(model.Nodes)!=1||model.Nodes[0].Components[0].State!=ComponentVerified||model.Nodes[0].Components[0].Version!="v1.2.3"{
  t.Fatalf("signed catalog not reflected in API: %+v",model)
 }
 if x:=CompareVerifiedComponentDigest(hex.EncodeToString(digest[:]),got,"baft-linux-amd64");x!=ComponentVerified{t.Fatalf("signed artifact status %s",x)}
 // A root-signed but older revocation envelope must never be accepted.
 older,err:=release.SignRevocations(rootPriv,1,nil,now.Add(-time.Hour),now.Add(24*time.Hour));if err!=nil{t.Fatal(err)}
 if err:=release.WriteEnvelope(revPath,older);err!=nil{t.Fatal(err)}
 if _,err:=cat.Verify(now);err==nil{t.Fatal("older revocation sequence accepted")}
 response=httptest.NewRecorder()
 app.Handler().ServeHTTP(response,authReq(http.MethodGet,"/api/nodes/versions","admin-secret",nil))
 if response.Code!=http.StatusOK{t.Fatalf("fail-closed endpoint status=%d",response.Code)}
 var unavailable struct {
  CatalogStatus string `json:"catalog_status"`
  Nodes []NodeVersionStatus `json:"nodes"`
 }
 if err:=json.Unmarshal(response.Body.Bytes(),&unavailable);err!=nil{t.Fatal(err)}
 if unavailable.CatalogStatus!="unavailable"||len(unavailable.Nodes)!=1{t.Fatalf("unexpected failure model: %+v",unavailable)}
 for _,component:=range unavailable.Nodes[0].Components {
  if component.State!=ComponentUnknown||component.Version!=""{t.Fatalf("untrusted version leaked after revocation replay: %+v",component)}
 }
 if _,err:=NewVerifiedReleaseCatalog(dir,rootPath,filepath.Join(dir,"revocations.json"),trustPath);err==nil{t.Fatal("revocations inside signed artifacts accepted")}
 if _,err:=NewVerifiedReleaseCatalog(dir,rootPath,revPath,filepath.Join(dir,"trust.json"));err==nil{t.Fatal("trust state inside signed artifacts accepted")}
 alias:=filepath.Join(base,"signed-alias")
 if err:=os.Symlink(dir,alias);err!=nil{t.Fatal(err)}
 if _,err:=NewVerifiedReleaseCatalog(dir,rootPath,revPath,filepath.Join(alias,"trust.json"));err==nil{t.Fatal("symlinked trust state inside signed artifacts accepted")}
 revAlias:=filepath.Join(base,"rev-alias.json")
 if err:=os.Symlink(filepath.Join(dir,"manifest.json"),revAlias);err!=nil{t.Fatal(err)}
 if _,err:=NewVerifiedReleaseCatalog(dir,rootPath,revAlias,trustPath);err==nil{t.Fatal("symlinked revocations inside signed artifacts accepted")}
 stateAlias:=filepath.Join(base,"state-alias.json")
 if err:=os.Symlink(filepath.Join(dir,"manifest.json"),stateAlias);err!=nil{t.Fatal(err)}
 if _,err:=NewVerifiedReleaseCatalog(dir,rootPath,revPath,stateAlias);err==nil{t.Fatal("symlinked trust-state file inside signed artifacts accepted")}
}

func TestVerifiedReleaseCatalogRequiresValidPinnedRoot(t *testing.T) {
 base:=t.TempDir()
 rootPath:=filepath.Join(base,"root.pub")
 if err:=os.WriteFile(rootPath,[]byte("not-a-key"),0600);err!=nil{t.Fatal(err)}
 if _,err:=NewVerifiedReleaseCatalog(filepath.Join(base,"signed"),rootPath,filepath.Join(base,"rev.json"),filepath.Join(base,"trust.json"));err==nil{t.Fatal("invalid pinned root accepted")}
}

func TestVerifiedReleaseCatalogSeesRotatedRevocationSymlink(t *testing.T) {
 base:=t.TempDir()
 dir:=filepath.Join(base,"signed")
 if err:=os.Mkdir(dir,0700);err!=nil{t.Fatal(err)}
 now:=time.Date(2026,10,18,16,0,0,0,time.UTC)
 rootPub,rootPriv,err:=release.GenerateKey();if err!=nil{t.Fatal(err)}
 signPub,signPriv,err:=release.GenerateKey();if err!=nil{t.Fatal(err)}
 cert,err:=release.Certify(rootPriv,signPub,now.Add(-time.Hour),now.Add(time.Hour));if err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(dir,"baft-linux-amd64"),[]byte("binary"),0700);err!=nil{t.Fatal(err)}
 if _,err:=release.SignDir(release.SignInput{Dir:dir,Version:"v1.0.0",Commit:strings.Repeat("a",40),CreatedAt:now,Provenance:release.Provenance{Builder:"test",GoVersion:"go1.27"},Key:signPriv,Cert:cert,Root:rootPub});err!=nil{t.Fatal(err)}
 rootPath:=filepath.Join(base,"root.pub")
 if err:=os.WriteFile(rootPath,[]byte(release.EncodePublic(rootPub)),0600);err!=nil{t.Fatal(err)}
 first,err:=release.SignRevocations(rootPriv,1,nil,now.Add(-time.Hour),now.Add(time.Hour));if err!=nil{t.Fatal(err)}
 second,err:=release.SignRevocations(rootPriv,2,[]string{release.KeyID(signPub)},now.Add(-time.Hour),now.Add(time.Hour));if err!=nil{t.Fatal(err)}
 rev1:=filepath.Join(base,"rev-1.json")
 rev2:=filepath.Join(base,"rev-2.json")
 if err:=release.WriteEnvelope(rev1,first);err!=nil{t.Fatal(err)}
 if err:=release.WriteEnvelope(rev2,second);err!=nil{t.Fatal(err)}
 link:=filepath.Join(base,"current-revocations.json")
 if err:=os.Symlink(rev1,link);err!=nil{t.Fatal(err)}
 cat,err:=NewVerifiedReleaseCatalog(dir,rootPath,link,filepath.Join(base,"trust.json"))
 if err!=nil{t.Fatal(err)}
 if _,err:=cat.Verify(now);err!=nil{t.Fatalf("initial valid catalog: %v",err)}
 replacement:=filepath.Join(base,"new-link")
 if err:=os.Symlink(rev2,replacement);err!=nil{t.Fatal(err)}
 if err:=os.Rename(replacement,link);err!=nil{t.Fatal(err)}
 if _,err:=cat.Verify(now);err==nil{t.Fatal("rotated revocations still accepted revoked signing key")}
}

func TestVerifiedReleaseCatalogSeesRotatedDirectorySymlink(t *testing.T) {
 base:=t.TempDir()
 now:=time.Date(2026,10,18,16,0,0,0,time.UTC)
 rootPub,rootPriv,err:=release.GenerateKey();if err!=nil{t.Fatal(err)}
 signPub,signPriv,err:=release.GenerateKey();if err!=nil{t.Fatal(err)}
 cert,err:=release.Certify(rootPriv,signPub,now.Add(-time.Hour),now.Add(time.Hour));if err!=nil{t.Fatal(err)}
 rootPath:=filepath.Join(base,"root.pub")
 if err:=os.WriteFile(rootPath,[]byte(release.EncodePublic(rootPub)),0600);err!=nil{t.Fatal(err)}
 rev,err:=release.SignRevocations(rootPriv,1,nil,now.Add(-time.Hour),now.Add(time.Hour));if err!=nil{t.Fatal(err)}
 revPath:=filepath.Join(base,"revocations.json")
 if err:=release.WriteEnvelope(revPath,rev);err!=nil{t.Fatal(err)}
 makeDir:=func(name,version,commit string)string{
  t.Helper()
  dir:=filepath.Join(base,name)
  if err:=os.Mkdir(dir,0700);err!=nil{t.Fatal(err)}
  if err:=os.WriteFile(filepath.Join(dir,"baft-linux-amd64"),[]byte(version),0700);err!=nil{t.Fatal(err)}
  if _,err:=release.SignDir(release.SignInput{Dir:dir,Version:version,Commit:strings.Repeat(commit,40),CreatedAt:now,Provenance:release.Provenance{Builder:"test",GoVersion:"go1.27"},Key:signPriv,Cert:cert,Root:rootPub});err!=nil{t.Fatal(err)}
  return dir
 }
 first:=makeDir("first","v1.0.0","a")
 second:=makeDir("second","v1.1.0","b")
 current:=filepath.Join(base,"current")
 if err:=os.Symlink(first,current);err!=nil{t.Fatal(err)}
 cat,err:=NewVerifiedReleaseCatalog(current,rootPath,revPath,filepath.Join(base,"trust.json"));if err!=nil{t.Fatal(err)}
 v,err:=cat.Verify(now);if err!=nil||v.Version!="v1.0.0"{t.Fatalf("first version: %v %+v",err,v)}
 next:=filepath.Join(base,"next")
 if err:=os.Symlink(second,next);err!=nil{t.Fatal(err)}
 if err:=os.Rename(next,current);err!=nil{t.Fatal(err)}
 v,err=cat.Verify(now);if err!=nil||v.Version!="v1.1.0"{t.Fatalf("rotated catalog not observed: %v %+v",err,v)}
 state,err:=release.ReadState(filepath.Join(base,"trust.json"));if err!=nil{t.Fatal(err)}
 if state.Version!=""||state.RevocationSequence!=1{t.Fatalf("read-only observation ratcheted accepted version: %+v",state)}
}

func TestVerifiedReleaseCatalogReadDoesNotAdvanceAcceptedRelease(t *testing.T) {
 base:=t.TempDir()
 dir:=filepath.Join(base,"signed")
 if err:=os.Mkdir(dir,0700);err!=nil{t.Fatal(err)}
 now:=time.Date(2026,10,18,16,0,0,0,time.UTC)
 rootPub,rootPriv,err:=release.GenerateKey();if err!=nil{t.Fatal(err)}
 signPub,signPriv,err:=release.GenerateKey();if err!=nil{t.Fatal(err)}
 cert,err:=release.Certify(rootPriv,signPub,now.Add(-time.Hour),now.Add(time.Hour));if err!=nil{t.Fatal(err)}
 rev,err:=release.SignRevocations(rootPriv,4,nil,now.Add(-time.Hour),now.Add(time.Hour));if err!=nil{t.Fatal(err)}
 rootPath:=filepath.Join(base,"root.pub")
 revPath:=filepath.Join(base,"revocations.json")
 statePath:=filepath.Join(base,"trust.json")
 if err:=os.WriteFile(rootPath,[]byte(release.EncodePublic(rootPub)),0600);err!=nil{t.Fatal(err)}
 if err:=release.WriteEnvelope(revPath,rev);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(dir,"baft-linux-amd64"),[]byte("new release"),0700);err!=nil{t.Fatal(err)}
 if _,err:=release.SignDir(release.SignInput{Dir:dir,Version:"v2.0.0",Commit:strings.Repeat("b",40),CreatedAt:now,Provenance:release.Provenance{Builder:"test",GoVersion:"go1.27"},Key:signPriv,Cert:cert,Root:rootPub});err!=nil{t.Fatal(err)}
 accepted:=release.TrustState{SchemaVersion:1,Version:"v1.0.0",Commit:strings.Repeat("a",40),RevocationSequence:3}
 if err:=release.WriteState(statePath,accepted);err!=nil{t.Fatal(err)}
 cat,err:=NewVerifiedReleaseCatalog(dir,rootPath,revPath,statePath);if err!=nil{t.Fatal(err)}
 for i:=0;i<2;i++ {
  v,err:=cat.Verify(now);if err!=nil||v.Version!="v2.0.0"{t.Fatalf("verified observation %d: %v %+v",i,err,v)}
 }
 state,err:=release.ReadState(statePath);if err!=nil{t.Fatal(err)}
 if state.Version!="v1.0.0"||state.Commit!=accepted.Commit||state.RevocationSequence!=4 {
  t.Fatalf("read ratcheted accepted version or failed revocation floor: %+v",state)
 }
}
