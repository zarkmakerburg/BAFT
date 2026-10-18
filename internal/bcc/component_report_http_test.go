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
 srv := &Server{store:store, now:func()time.Time{return now}}
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
