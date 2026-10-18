package bcc

import (
 "path/filepath"
 "testing"
 "time"
)

func TestComponentDigestReceiptSQLiteReopen(t *testing.T) {
 path := filepath.Join(t.TempDir(), "bcc.sqlite")
 now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
 original := state{ComponentDigestReceipts: map[string]ComponentDigestReceipt{
  "node-1": {NodeID: "node-1", ObservedAt: now, AcceptedAt: now, Digests: map[string]string{"transport": "unknown"}},
 }}
 normalizeState(&original)
 if err := writeStateDB(path, original); err != nil { t.Fatal(err) }
 loaded, err := readStateDB(path)
 if err != nil { t.Fatal(err) }
 got, ok := loaded.ComponentDigestReceipts["node-1"]
 if !ok { t.Fatal("receipt lost after reopen") }
 if got.NodeID != "node-1" || !got.ObservedAt.Equal(now) || got.Digests["transport"] != "unknown" { t.Fatalf("unexpected persisted receipt: %+v", got) }
}
