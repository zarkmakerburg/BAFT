package bcc

import (
 "errors"
 "strings"
 "time"
)

// AcceptComponentDigestReceipt stores a node-authenticated receipt and its replay
// cursor in the same SQLite state transaction. It does NOT attest a release
// version: that requires a separately verified signed release manifest.
func (s *Store) AcceptComponentDigestReceipt(receipt ComponentDigestReceipt, token string, now time.Time, maxAge time.Duration) error {
 if strings.TrimSpace(receipt.NodeID) == "" || len(receipt.NodeID) > 128 { return ErrAgentAuthentication }
 if now.IsZero() { return errors.New("current time required") }
 if len(receipt.Digests) == 0 || len(receipt.Digests) > 3 { return errors.New("invalid component digest inventory") }
 for component, digest := range receipt.Digests {
  if component != "transport" && component != "agent" && component != "bcc" { return errors.New("unexpected component") }
  if digest != "unknown" && !isHexSHA256(digest) { return errors.New("invalid component digest") }
 }
 s.mu.Lock()
 defer s.mu.Unlock()
 if _, ok := s.authorizedHashLocked(receipt.NodeID, token, now); !ok { return ErrAgentAuthentication }
 previous := s.st.ComponentDigestReceipts[receipt.NodeID]
 if err := CheckComponentReceiptFreshness(receipt.ObservedAt, previous.ObservedAt, now, maxAge); err != nil { return err }
 before, err := cloneState(s.st)
 if err != nil { return err }
 if s.st.ComponentDigestReceipts == nil { s.st.ComponentDigestReceipts = make(map[string]ComponentDigestReceipt) }
 receipt.AcceptedAt = now.UTC()
 s.st.ComponentDigestReceipts[receipt.NodeID] = receipt
 if err := s.saveLocked(); err != nil { s.st = before; return err }
 return nil
}

func isHexSHA256(s string) bool {
 if len(s) != 64 { return false }
 for _, c := range s { if (c < '0' || c > '9') && (c < 'a' || c > 'f') { return false } }
 return true
}
