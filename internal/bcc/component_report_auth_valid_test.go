package bcc

import (
 "errors"
 "testing"
 "time"
)

func TestAuthorizeComponentReceiptValidAndRevoked(t *testing.T) {
 now := time.Now().UTC()
 token := "component-report-test-token"
 s := &Store{st: state{Nodes: map[string]Node{
  "node-1": {ID: "node-1", AgentTokenHash: tokenHash(token)},
 }}}
 if err := s.AuthorizeComponentReceipt("node-1", token, now, time.Time{}, now, time.Minute); err != nil { t.Fatalf("valid token rejected: %v", err) }
 if err := s.AuthorizeComponentReceipt("node-1", "incorrect", now, time.Time{}, now, time.Minute); !errors.Is(err, ErrAgentAuthentication) { t.Fatalf("wrong token accepted: %v", err) }
 n := s.st.Nodes["node-1"]
 n.Revoked = true
 s.st.Nodes["node-1"] = n
 if err := s.AuthorizeComponentReceipt("node-1", token, now, time.Time{}, now, time.Minute); !errors.Is(err, ErrAgentAuthentication) { t.Fatalf("revoked node accepted: %v", err) }
}
