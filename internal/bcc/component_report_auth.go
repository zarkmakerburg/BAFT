package bcc

import (
 "errors"
 "strings"
 "time"
)

// AuthorizeComponentReceipt is a preflight guard for an eventual BCC receiver.
// It does not store the report or establish manifest trust.
func (s *Store) AuthorizeComponentReceipt(nodeID, token string, observedAt, lastAccepted, now time.Time, maxAge time.Duration) error {
 if strings.TrimSpace(nodeID) == "" { return ErrAgentAuthentication }
 s.mu.Lock()
 _, ok := s.authorizedHashLocked(nodeID, token, now)
 s.mu.Unlock()
 if !ok { return ErrAgentAuthentication }
 if err := CheckComponentReceiptFreshness(observedAt, lastAccepted, now, maxAge); err != nil { return err }
 if now.IsZero() { return errors.New("current time required") }
 return nil
}
