package bcc

import (
 "net/http"
 "net/http/httptest"
 "path/filepath"
 "testing"
 "time"
)

func TestNodePlatformAdminConfirmationPersistenceAndInvalidation(t *testing.T) {
 path:=filepath.Join(t.TempDir(),"state.db")
 store,err:=OpenStore(path)
 if err!=nil{t.Fatal(err)}
 initial:=Node{ID:"ex-1",Address:"127.0.0.1:2443",PathIPv4:"127.0.0.1",Role:"foreign",PublicKey:"key-a",Alias:"A"}
 if _,err:=store.UpsertNode(initial,"agent-secret");err!=nil{t.Fatal(err)}
 app,err:=NewServer(store,"admin-secret")
 if err!=nil{t.Fatal(err)}
 now:=time.Date(2026,10,18,14,0,0,0,time.UTC)
 app.now=func()time.Time{return now}
 post:=func(token,ack,goos,arch string) int {
  w:=httptest.NewRecorder()
  app.Handler().ServeHTTP(w,authReq(http.MethodPost,"/api/nodes/platform",token,map[string]any{
   "node_id":"ex-1","os":goos,"arch":arch,"confirmation":ack,
  }))
  return w.Code
 }
 if got:=post("","verified-out-of-band","linux","amd64");got==http.StatusOK{t.Fatal("unauthenticated confirmation accepted")}
 if got:=post("admin-secret","","linux","amd64");got!=http.StatusBadRequest{t.Fatalf("confirmation without acknowledgement=%d",got)}
 if got:=post("admin-secret","verified-out-of-band","windows","amd64");got!=http.StatusBadRequest{t.Fatalf("invalid OS=%d",got)}
 if got:=post("admin-secret","verified-out-of-band","linux","amd64");got!=http.StatusOK{t.Fatalf("valid confirmation=%d",got)}
 n,ok:=store.GetNode("ex-1")
 if !ok||!n.Platform.Confirmed||n.Platform.Source!="operator-verified"||n.Platform.OS!="linux"||n.Platform.Arch!="amd64"||!n.Platform.ConfirmedAt.Equal(now){t.Fatalf("invalid confirmation record: %+v",n.Platform)}
 reopened,err:=OpenStore(path)
 if err!=nil{t.Fatal(err)}
 n,ok=reopened.GetNode("ex-1")
 if !ok||!n.Platform.Confirmed{t.Fatal("platform evidence lost on restart")}
 initial.Alias="Renamed"
 // A partial API update may omit the route that BCC retains from the
 // existing node. It must not silently invalidate confirmation.
 initial.PathIPv4=""
 if _,err:=reopened.UpsertNode(initial,"");err!=nil{t.Fatal(err)}
 n,_=reopened.GetNode("ex-1")
 if !n.Platform.Confirmed{t.Fatal("cosmetic alias edit unexpectedly invalidated platform")}
 initial.Address="127.0.0.2:2443"
 if _,err:=reopened.UpsertNode(initial,"");err!=nil{t.Fatal(err)}
 n,_=reopened.GetNode("ex-1")
 if n.Platform.Confirmed{t.Fatal("node rebind retained stale platform evidence")}
 initial.Platform=NodePlatformEvidence{OS:"linux",Arch:"amd64",Confirmed:true,Source:"operator-verified"}
 if _,err:=reopened.UpsertNode(initial,"");err!=nil{t.Fatal(err)}
 n,_=reopened.GetNode("ex-1")
 if n.Platform.Confirmed{t.Fatal("generic node upsert forged confirmation")}
}

func TestNodePlatformRevokeClearsConfirmation(t *testing.T) {
 store,err:=OpenStore(filepath.Join(t.TempDir(),"state.db"))
 if err!=nil{t.Fatal(err)}
 if _,err:=store.UpsertNode(Node{ID:"node",Address:"127.0.0.1",Role:"worker"},"");err!=nil{t.Fatal(err)}
 now:=time.Now().UTC()
 if _,err:=store.ConfirmNodePlatform("node","linux","arm64",now,AuditEntry{Actor:"admin"});err!=nil{t.Fatal(err)}
 if _,err:=store.RevokeNode("node","retired",now);err!=nil{t.Fatal(err)}
 node,_:=store.GetNode("node")
 if node.Platform.Confirmed{t.Fatal("revoked node has trusted platform")}
 if _,err:=store.ConfirmNodePlatform("node","linux","arm64",now,AuditEntry{Actor:"admin"});err==nil{t.Fatal("revoked node reconfirmed")}
}
