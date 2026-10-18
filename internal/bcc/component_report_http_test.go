package bcc

import (
 "bytes"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "path/filepath"
 "testing"
 "time"
)

func TestAgentComponentReportHTTPAuthAndReplay(t *testing.T) {
 store, err := OpenStore(filepath.Join(t.TempDir(), "bcc.sqlite"))
 if err != nil { t.Fatal(err) }
 store.st.Nodes["node-1"] = Node{ID:"node-1", AgentTokenHash:tokenHash("test-token")}
 if err := store.saveLocked(); err != nil { t.Fatal(err) }
 now := time.Date(2026,10,18,12,0,0,0,time.UTC)
 srv, err := NewServer(store, "admin-test-secret")
 if err != nil { t.Fatal(err) }
 srv.now = func()time.Time{return now}
 report := map[string]any{"node_id":"node-1", "observed_at":now, "components":[]map[string]string{
  {"component":"transport", "state":"unknown"},
  {"component":"agent", "state":"unknown"},
  {"component":"bcc", "state":"unknown"},
 }}
 payload, err := json.Marshal(report)
 if err != nil { t.Fatal(err) }
 send := func(token string, body []byte) int {
  req := httptest.NewRequest(http.MethodPost,"/api/agent/component-report",bytes.NewReader(body))
  req.Header.Set("Authorization","Bearer "+token)
  rec := httptest.NewRecorder()
  srv.agentComponentReport(rec,req)
  return rec.Code
 }
 if got:=send("bad-token",payload);got!=http.StatusUnauthorized {t.Fatalf("wrong token status=%d",got)}
 if got:=send("test-token",payload);got!=http.StatusAccepted {t.Fatalf("first report status=%d",got)}
 if got:=send("test-token",payload);got==http.StatusAccepted {t.Fatal("replay accepted")}
 reopened,err:=OpenStore(store.path)
 if err!=nil {t.Fatal(err)}
 srv.store=reopened
 if got:=send("test-token",payload);got==http.StatusAccepted {t.Fatal("replay accepted after reopen")}
 report["components"] = []map[string]string{{"component":"transport","state":"unknown"},{"component":"transport","state":"unknown"},{"component":"bcc","state":"unknown"}}
 bad,err:=json.Marshal(report);if err!=nil{t.Fatal(err)}
 if got:=send("test-token",bad);got!=http.StatusBadRequest {t.Fatalf("duplicate component status=%d",got)}
}

func TestAgentComponentReportHTTPRejectsRevokedNodeAfterReopen(t *testing.T) {
 store,err:=OpenStore(filepath.Join(t.TempDir(),"bcc.sqlite"))
 if err!=nil { t.Fatal(err) }
 store.st.Nodes["revocable"] = Node{ID:"revocable",AgentTokenHash:tokenHash("valid-agent-token")}
 if err:=store.saveLocked();err!=nil { t.Fatal(err) }
 now:=time.Date(2026,10,18,12,0,0,0,time.UTC)
 srv,err:=NewServer(store,"admin-test-secret")
 if err!=nil { t.Fatal(err) }
 srv.now=func()time.Time{return now}
 send:=func(at time.Time)int {
  t.Helper()
  payload,err:=json.Marshal(map[string]any{"node_id":"revocable","observed_at":at,"components":[]map[string]string{
   {"component":"transport","state":"unknown"},{"component":"agent","state":"unknown"},{"component":"bcc","state":"unknown"},
  }})
  if err!=nil {t.Fatal(err)}
  req:=httptest.NewRequest(http.MethodPost,"/api/agent/component-report",bytes.NewReader(payload))
  req.Header.Set("Authorization","Bearer valid-agent-token")
  rr:=httptest.NewRecorder()
  srv.Handler().ServeHTTP(rr,req)
  return rr.Code
 }
 if got:=send(now);got!=http.StatusAccepted {t.Fatalf("pre-revocation report status=%d",got)}
 if _,err:=store.RevokeNode("revocable","security test",now.Add(time.Second));err!=nil {t.Fatal(err)}
 reopened,err:=OpenStore(store.path)
 if err!=nil {t.Fatal(err)}
 srv.store=reopened
 now=now.Add(2*time.Second)
 if got:=send(now);got!=http.StatusUnauthorized {t.Fatalf("revoked node accepted via production handler: HTTP %d",got)}
 _,receipts:=reopened.ComponentVersionSnapshot()
 if !receipts["revocable"].ObservedAt.Equal(now.Add(-2*time.Second)) {t.Fatalf("revoked node advanced receipt cursor: %+v",receipts["revocable"])}
}

func TestAgentComponentReportHTTPBoundedClockSkew(t *testing.T) {
 store,err:=OpenStore(filepath.Join(t.TempDir(),"bcc.sqlite"))
 if err!=nil {t.Fatal(err)}
 store.st.Nodes["skew-node"]=Node{ID:"skew-node",AgentTokenHash:tokenHash("skew-token")}
 if err:=store.saveLocked();err!=nil {t.Fatal(err)}
 now:=time.Date(2026,10,18,12,0,0,0,time.UTC)
 srv,err:=NewServer(store,"admin-secret")
 if err!=nil {t.Fatal(err)}
 srv.now=func()time.Time{return now}
 send:=func(at time.Time)int {
  t.Helper()
  body,err:=json.Marshal(map[string]any{"node_id":"skew-node","observed_at":at,"components":[]map[string]string{
   {"component":"transport","state":"unknown"},{"component":"agent","state":"unknown"},{"component":"bcc","state":"unknown"},
  }})
  if err!=nil {t.Fatal(err)}
  req:=httptest.NewRequest(http.MethodPost,"/api/agent/component-report",bytes.NewReader(body))
  req.Header.Set("Authorization","Bearer skew-token")
  rr:=httptest.NewRecorder()
  srv.Handler().ServeHTTP(rr,req)
  return rr.Code
 }
 first:=now.Add(50*time.Millisecond)
 if got:=send(first);got!=http.StatusAccepted {t.Fatalf("50ms future skew rejected: %d",got)}
 if got:=send(first);got==http.StatusAccepted {t.Fatal("replay accepted within skew tolerance")}
 if got:=send(now.Add(3*time.Minute));got==http.StatusAccepted {t.Fatal("unbounded future report accepted")}
 if got:=send(now.Add(time.Second));got!=http.StatusAccepted {t.Fatalf("ordered future report rejected: %d",got)}
}
