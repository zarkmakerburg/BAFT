package tunnelnode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRollbackRejectsCorruptInactiveTransaction(t *testing.T) {
	root := newNode(t)
	m, err := root.Manager.ForInstance("rollback-corrupt", "127.0.0.1:19371")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if result, err := m.Rollback(ctx, "not-created"); err != nil || result != "nothing to roll back" {
		t.Fatalf("unknown transaction should remain idempotent: result=%q err=%v", result, err)
	}

	const id = "corrupt-record"
	path := m.txnPath(id)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	const corrupted = "{broken-json"
	if err := os.WriteFile(path, []byte(corrupted), 0600); err != nil {
		t.Fatal(err)
	}
	if result, err := m.Rollback(ctx, id); err == nil || !strings.Contains(err.Error(), "reconciliation required") {
		t.Fatalf("corrupt inactive transaction falsely accepted: result=%q err=%v", result, err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != corrupted {
		t.Fatalf("rollback changed corrupt evidence: got=%q err=%v", got, err)
	}
}
