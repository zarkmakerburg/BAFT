package bcc

import (
 "errors"
 "testing"
 "time"
)

func TestAuthorizeComponentReceiptRejectsUnknownNode(t *testing.T) {
 s := &Store{st: state{Nodes: map[string]Node{}}}
 now := time.Now().UTC()
 err := s.AuthorizeComponentReceipt("unknown", "bad-token", now, time.Time{}, now, time.Minute)
 if !errors.Is(err, ErrAgentAuthentication) { t.Fatalf("expected authentication failure, got %v", err) }
}

func TestAuthorizeComponentReceiptRejectsEmptyNode(t *testing.T) {
 s := &Store{st: state{Nodes: map[string]Node{}}}
 now := time.Now().UTC()
 err := s.AuthorizeComponentReceipt(" ", "token", now, time.Time{}, now, time.Minute)
 if !errors.Is(err, ErrAgentAuthentication) { t.Fatalf("expected authentication failure, got %v", err) }
}
