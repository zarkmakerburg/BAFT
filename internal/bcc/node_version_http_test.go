package bcc

import (
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strings"
 "path/filepath"
 "testing"
)

func TestNodeVersionsEndpointUnknownWithoutSignedCatalog(t *testing.T) {
 store,err:=OpenStore(filepath.Join(t.TempDir(),"state.db"))
 if err!=nil{t.Fatal(err)}
 if _,err:=store.UpsertNode(Node{ID:"ex-1",Address:"127.0.0.1:2443",Role:"foreign"},"agent");err!=nil{t.Fatal(err)}
 app,err:=NewServer(store,"admin-secret")
 if err!=nil{t.Fatal(err)}
 makeRequest:=func(token string)*httptest.ResponseRecorder{
  rec:=httptest.NewRecorder()
  app.Handler().ServeHTTP(rec,authReq(http.MethodGet,"/api/nodes/versions",token,nil))
  return rec
 }
 if rec:=makeRequest("");rec.Code==http.StatusOK{t.Fatal("unauthorized read accepted")}
 for _,method:=range []string{http.MethodPost,http.MethodPut,http.MethodDelete} {
  forbidden:=httptest.NewRecorder()
  app.Handler().ServeHTTP(forbidden,authReq(method,"/api/nodes/versions","admin-secret",strings.NewReader("{}")))
  if forbidden.Code!=http.StatusMethodNotAllowed{t.Fatalf("method %s accepted: %d",method,forbidden.Code)}
 }
 rec:=makeRequest("admin-secret")
 if rec.Code!=http.StatusOK{t.Fatalf("status %d: %s",rec.Code,rec.Body.String())}
 var response struct {
  CatalogConfigured bool `json:"catalog_configured"`
  CatalogStatus string `json:"catalog_status"`
  Nodes []NodeVersionStatus `json:"nodes"`
 }
 if err:=json.Unmarshal(rec.Body.Bytes(),&response);err!=nil{t.Fatal(err)}
 if response.CatalogConfigured||response.CatalogStatus!="not_configured"||len(response.Nodes)!=1||len(response.Nodes[0].Components)!=3{t.Fatalf("unexpected model: %+v",response)}
 for _,item:=range response.Nodes[0].Components {
  if item.State!=ComponentUnknown||item.Version!=""{t.Fatalf("unverified version exposed: %+v",item)}
 }
}

func TestNodeVersionSnapshotCopiesDigests(t *testing.T) {
 store,err:=OpenStore(filepath.Join(t.TempDir(),"state.db"))
 if err!=nil{t.Fatal(err)}
 store.mu.Lock()
 store.st.ComponentDigestReceipts=map[string]ComponentDigestReceipt{"node":{NodeID:"node",Digests:map[string]string{"agent":"unknown"}}}
 store.mu.Unlock()
 _,items:=store.ComponentVersionSnapshot()
 rec:=items["node"]
 rec.Digests["agent"]="modified"
 _,again:=store.ComponentVersionSnapshot()
 if again["node"].Digests["agent"]!="unknown"{t.Fatal("returned map aliases persistent digest state")}
}
