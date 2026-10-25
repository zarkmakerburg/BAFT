package bcc

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The documented operator path: offline bundle directory -> update_center_bundle.sh -> import.
func TestUpdateCenterBundleScriptOutputImports(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	e := newUCEnv(t, "v0.4.0")
	b := e.f.bundle("v0.4.1", "b", e.f.revocations(1))
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "release"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, v := range map[string]any{"release/manifest.json": b.Manifest, "release/release-key.cert.json": b.Cert, "revocations.json": b.Revocations} {
		if err := os.WriteFile(filepath.Join(dir, name), ucJSON(t, v), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "release/SHA256SUMS"), []byte(b.SHA256SUMS), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("bash", "../../scripts/release/update_center_bundle.sh", dir).Output()
	if err != nil {
		t.Fatalf("script: %v", err)
	}
	mustCode(t, e.do(http.MethodPost, "/api/updates/import", "admin-secret", out), http.StatusCreated)
	rev, err := exec.Command("bash", "../../scripts/release/update_center_bundle.sh", "--revocations-only", filepath.Join(dir, "revocations.json")).Output()
	if err != nil {
		t.Fatalf("script: %v", err)
	}
	mustCode(t, e.do(http.MethodPost, "/api/updates/import", "admin-secret", rev), http.StatusOK)
}
