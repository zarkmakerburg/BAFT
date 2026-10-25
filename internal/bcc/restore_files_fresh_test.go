package bcc

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A backup that carries audit entries must restore onto a host whose audit log
// is empty (fresh install / wiped state), and must still be refused when the
// current audit is non-empty and does not extend the backup anchor.
func TestRestoreFilesOntoEmptyAuditAndRefusesDivergentAudit(t *testing.T) {
	root := t.TempDir()
	key := backupTestKey()
	at := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

	srcState := filepath.Join(root, "src.db")
	store, err := OpenStore(srcState)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertNode(Node{ID: "n-src", Alias: "Src", Address: "127.0.0.1:21001", Role: "foreign"}, "tok"); err != nil {
		t.Fatal(err)
	}
	app, err := NewServer(store, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.audit.Append(AuditEntry{Timestamp: at, Actor: "admin", Action: "node.create", Target: "n-src", Outcome: "success"}); err != nil {
		t.Fatal(err)
	}
	backupFile := filepath.Join(root, "b.baftbak")
	h, err := app.BackupToFile(backupFile, key, at)
	if err != nil {
		t.Fatal(err)
	}
	if h.AuditSequence == 0 {
		t.Fatal("test needs a backup with a non-empty audit chain")
	}

	fresh := filepath.Join(root, "fresh.db")
	if err := RestoreFiles(fresh, backupFile, key, at.Add(time.Minute)); err != nil {
		t.Fatalf("restore onto an empty audit log must work: %v", err)
	}
	reopened, err := OpenStore(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if n := reopened.ListNodes(); len(n) != 1 || n[0].ID != "n-src" {
		t.Fatalf("nodes after restore onto fresh host=%+v", n)
	}

	other := filepath.Join(root, "other.db")
	ostore, err := OpenStore(other)
	if err != nil {
		t.Fatal(err)
	}
	oapp, err := NewServer(ostore, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := oapp.audit.Append(AuditEntry{Timestamp: at, Actor: "someone", Action: "node.create", Target: "x", Outcome: "success"}); err != nil {
		t.Fatal(err)
	}
	err = RestoreFiles(other, backupFile, key, at.Add(time.Minute))
	if err == nil || !strings.Contains(err.Error(), "does not extend backup anchor") {
		t.Fatalf("divergent audit must be refused, err=%v", err)
	}
}
