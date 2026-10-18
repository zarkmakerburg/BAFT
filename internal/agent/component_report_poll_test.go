package agent

import (
 "context"
 "net/http"
 "net/http/httptest"
 "testing"
 "time"
)

func TestComponentReportPollThrottleAndErrorRecovery(t *testing.T) {
 now := time.Date(2026,10,18,12,0,0,0,time.UTC)
 attempts := 0
 status := http.StatusServiceUnavailable
 server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request) {
  if r.URL.Path != "/api/agent/component-report" { t.Errorf("unexpected path: %s",r.URL.Path) }
  attempts++
  w.WriteHeader(status)
 }))
 defer server.Close()
 a := &Agent{cfg:Config{BCCURL:server.URL, NodeID:"node-1", Token:"test-token", BinDir:t.TempDir(), HTTP:server.Client(), Now:func()time.Time{return now}}}
 a.maybeSendComponentReport(context.Background())
 if attempts!=1 {t.Fatalf("first attempts=%d",attempts)}
 _,lastErr:=a.ComponentReportStatus()
 if lastErr=="" {t.Fatal("failed report not observable")}
 now=now.Add(time.Minute)
 a.maybeSendComponentReport(context.Background())
 if attempts!=1 {t.Fatalf("throttle failed: attempts=%d",attempts)}
 now=now.Add(4*time.Minute)
 status=http.StatusAccepted
 a.maybeSendComponentReport(context.Background())
 if attempts!=2 {t.Fatalf("retry attempts=%d",attempts)}
 at,lastErr:=a.ComponentReportStatus()
 if !at.Equal(now) || lastErr!="" {t.Fatalf("recovery status: %s %q",at,lastErr)}
}
