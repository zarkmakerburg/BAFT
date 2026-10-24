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

// TestGammaStoppedBCCRestoreCLI exercises the compiled CLI through independent
// processes, with a real cross-process state lock and encrypted backup. It
// never touches a system service or another user's state.
func TestGammaStoppedBCCRestoreCLI(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "baft-bcc")
	build := exec.Command("go", "build", "-o", binary, "./cmd/baft-bcc")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build real baft-bcc binary: %v: %s", err, out)
	}
	state := filepath.Join(dir, "bcc-state.db")
	backup := filepath.Join(dir, "sealed.baftbak")
	key := bytes.Repeat([]byte{0x53}, 32)
	store, err := bcc.OpenStore(state)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.UpsertNode(bcc.Node{ID: "gamma-original", Alias: "Original", Address: "127.0.0.1:33001", Role: "foreign"}, "token-original")
	if err != nil {
		t.Fatal(err)
	}
	app, err := bcc.NewServer(store, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.BackupToFile(backup, key, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	_, err = store.UpsertNode(bcc.Node{ID: "gamma-later", Alias: "Later", Address: "127.0.0.1:33002", Role: "foreign"}, "token-later")
	if err != nil {
		t.Fatal(err)
	}

	goodKey := base64.StdEncoding.EncodeToString(key)
	run := func(candidate string, args ...string) ([]byte, error) {
		t.Helper()
		cmd := exec.Command(binary, append([]string{"restore", "--backup", backup, "--state-file", state}, args...)...)
		cmd.Env = append(os.Environ(), "BAFT_BCC_BACKUP_KEY="+candidate)
		data, err := cmd.CombinedOutput()
		if bytes.Contains(data, []byte(goodKey)) || bytes.Contains(data, []byte("token-original")) {
			t.Fatal("sensitive backup key or node token leaked into CLI output")
		}
		return data, err
	}
	assertTwoNodes := func(phase string) {
		t.Helper()
		restored, err := bcc.OpenStore(state)
		if err != nil {
			t.Fatal(err)
		}
		if got := len(restored.ListNodes()); got != 2 {
			t.Fatalf("%s mutated live state; nodes=%d want 2", phase, got)
		}
	}

	if out, err := run(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x24}, 32)), "--yes"); err == nil {
		t.Fatalf("wrong-key CLI restore accepted: %s", out)
	}
	assertTwoNodes("wrong key")
	release, err := bcc.LockState(state)
	if err != nil {
		t.Fatal(err)
	}
	out, busyErr := run(goodKey, "--yes")
	release()
	if busyErr == nil {
		t.Fatalf("CLI restored while active process held BCC lock: %s", out)
	}
	assertTwoNodes("busy lock")

	out, err = run(goodKey, "--yes", "--json")
	if err != nil || !strings.Contains(string(out), "\"restored\":true") {
		t.Fatalf("stopped BCC restore failed: err=%v output=%s", err, out)
	}
	restored, err := bcc.OpenStore(state)
	if err != nil {
		t.Fatal(err)
	}
	nodes := restored.ListNodes()
	if len(nodes) != 1 || nodes[0].ID != "gamma-original" {
		t.Fatalf("restored wrong generation of state: %+v", nodes)
	}
	if info, err := os.Stat(backup); err != nil || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("encrypted backup permissions: info=%v err=%v", info, err)
	}
}
