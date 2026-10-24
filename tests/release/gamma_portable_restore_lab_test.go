package release_test

import (
	"bytes"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/bcc"
)

// Fresh-directory transfer is only a LAB surrogate for moving a signed,
// encrypted BCC backup between machines. No actual two-host restore is claimed.
func TestGammaEncryptedBackupRestoreToFreshDirectory(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source", "state.db")
	destinationPath := filepath.Join(dir, "other-node", "state.db")
	backupPath := filepath.Join(dir, "transport", "snapshot.baftbak")
	key := bytes.Repeat([]byte{0x79}, 32)
	store, err := bcc.OpenStore(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertNode(bcc.Node{
		ID: "gamma-transfer", Alias: "Transfer", Address: "127.0.0.1:34519", Role: "foreign",
	}, "secret-gamma-transfer-token"); err != nil {
		t.Fatal(err)
	}
	server, err := bcc.NewServer(store, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.BackupToFile(backupPath, key, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "baft-bcc")
	cmdBuild := exec.Command("go", "build", "-o", bin, "./cmd/baft-bcc")
	cmdBuild.Dir = root
	if out, err := cmdBuild.CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	run := func(yes bool) (string, error) {
		t.Helper()
		args := []string{"restore", "--backup", backupPath, "--state-file", destinationPath, "--json"}
		if yes {
			args = append(args, "--yes")
		}
		cmd := exec.Command(bin, args...)
		cmd.Env = append(os.Environ(), "BAFT_BCC_BACKUP_KEY="+base64.StdEncoding.EncodeToString(key))
		out, err := cmd.CombinedOutput()
		if bytes.Contains(out, []byte("secret-gamma-transfer-token")) ||
			bytes.Contains(out, []byte(base64.StdEncoding.EncodeToString(key))) {
			t.Fatal("secrets leaked into restore output")
		}
		return string(out), err
	}
	// A destination administrator first creates the private state directory.
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o700); err != nil {
		t.Fatal(err)
	}
	out, err := run(false)
	if err == nil || !strings.Contains(out, "\"confirmation_required\":true") {
		t.Fatalf("preflight should require confirmation, err=%v: %s", err, out)
	}
	if _, err := os.Stat(destinationPath); !os.IsNotExist(err) {
		t.Fatalf("preview created destination database: %v", err)
	}
	out, err = run(true)
	if err != nil || !strings.Contains(out, "\"restored\":true") {
		t.Fatalf("fresh-directory CLI restore failed, err=%v: %s", err, out)
	}
	other, err := bcc.OpenStore(destinationPath)
	if err != nil {
		t.Fatal(err)
	}
	nodes := other.ListNodes()
	if len(nodes) != 1 || nodes[0].ID != "gamma-transfer" {
		t.Fatalf("independent destination lost the backed-up node: %+v", nodes)
	}
	original, err := bcc.OpenStore(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if nodes := original.ListNodes(); len(nodes) != 1 || nodes[0].ID != "gamma-transfer" {
		t.Fatalf("source mutated by independent restore: %+v", nodes)
	}
}
