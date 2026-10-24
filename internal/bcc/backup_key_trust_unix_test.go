//go:build unix

package bcc

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupKeyFromFileRejectsUntrustedFilesystemObjects(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "backup.key")
	content := []byte(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32)) + "\n")
	if err := os.WriteFile(original, content, 0600); err != nil {
		t.Fatal(err)
	}
	if key, err := BackupKeyFromFile(original); err != nil || len(key) != 32 {
		t.Fatalf("secure key rejected: %v", err)
	}

	t.Run("world-readable", func(t *testing.T) {
		if err := os.Chmod(original, 0644); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(original, 0600)
		if _, err := BackupKeyFromFile(original); err == nil {
			t.Fatal("public key file admitted")
		}
	})
	t.Run("group-readable", func(t *testing.T) {
		if err := os.Chmod(original, 0640); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(original, 0600)
		if _, err := BackupKeyFromFile(original); err == nil {
			t.Fatal("group-readable key admitted")
		}
	})
	t.Run("symlink", func(t *testing.T) {
		if err := os.Symlink(original, filepath.Join(dir, "alias")); err != nil {
			t.Fatal(err)
		}
		if _, err := BackupKeyFromFile(filepath.Join(dir, "alias")); err == nil {
			t.Fatal("symlink alias admitted")
		}
	})
	t.Run("hardlink", func(t *testing.T) {
		if err := os.Link(original, filepath.Join(dir, "linked")); err != nil {
			t.Fatal(err)
		}
		if _, err := BackupKeyFromFile(original); err == nil {
			t.Fatal("multi-linked key admitted")
		}
	})
	t.Run("oversized", func(t *testing.T) {
		large := filepath.Join(dir, "large")
		if err := os.WriteFile(large, []byte(strings.Repeat("A", 65536)), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := BackupKeyFromFile(large); err == nil {
			t.Fatal("oversized key admitted")
		}
	})
}
