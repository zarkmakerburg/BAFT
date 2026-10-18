package agent

import (
 "os"
 "path/filepath"
 "testing"
)

func TestBinarySHA256RejectsSymlink(t *testing.T) {
 dir := t.TempDir()
 target := filepath.Join(dir, "real")
 link := filepath.Join(dir, "link")
 if err := os.WriteFile(target, []byte("data"), 0600); err != nil { t.Fatal(err) }
 if err := os.Symlink(target, link); err != nil { t.Skipf("symlinks unsupported: %v", err) }
 if _, err := BinarySHA256(link); err == nil { t.Fatal("symlink unexpectedly accepted") }
}
