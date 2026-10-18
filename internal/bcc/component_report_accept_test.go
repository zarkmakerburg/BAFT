package bcc

import (
 "path/filepath"
 "testing"
 "time"
)

func TestAcceptComponentDigestReceiptReplayAfterRestart(t *testing.T) {
 path := filepath.Join(t.TempDir(), "state.sqlite")
 s, err := OpenStore(path)
 if err != nil { t.Fatal(err) }
 token := "receipt-test-secret"
 s.st.Nodes["node-1"] = Node{ID:"node-1", AgentTokenHash:tokenHash(token)}
 if err := s.saveLocked(); err != nil { t.Fatal(err) }
 now := time.Now().UTC()
 report := ComponentDigestReceipt{NodeID:"node-1", ObservedAt:now, Digests:map[string]string{"transport":"unknown", "agent":"unknown", "bcc":"unknown"}}
 if err := s.AcceptComponentDigestReceipt(report, token, now, time.Minute); err != nil { t.Fatal(err) }
 reopened, err := OpenStore(path)
 if err != nil { t.Fatal(err) }
 if _, ok := reopened.st.ComponentDigestReceipts["node-1"]; !ok { t.Fatal("receipt not durable") }
 if err := reopened.AcceptComponentDigestReceipt(report, token, now, time.Minute); err == nil { t.Fatal("replay accepted after restart") }
 report.ObservedAt = now.Add(time.Second)
 if err := reopened.AcceptComponentDigestReceipt(report, "wrong", now.Add(time.Second), time.Minute); err == nil { t.Fatal("wrong token accepted") }
}
