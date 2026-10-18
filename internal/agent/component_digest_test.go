package agent

import (
 "crypto/sha256"
 "encoding/hex"
 "os"
 "path/filepath"
 "testing"
)

func TestBinarySHA256(t *testing.T) {
 dir := t.TempDir()
 path := filepath.Join(dir, "baft")
 data := []byte("baft-test-binary")
 if err := os.WriteFile(path, data, 0600); err != nil { t.Fatal(err) }
 got, err := BinarySHA256(path)
 if err != nil { t.Fatal(err) }
 sum := sha256.Sum256(data)
 if want := hex.EncodeToString(sum[:]); got != want { t.Fatalf("got %q want %q", got, want) }
}

func TestBinarySHA256RejectsDirectoryAndMissing(t *testing.T) {
 dir := t.TempDir()
 if _, err := BinarySHA256(dir); err == nil { t.Fatal("directory accepted") }
 if _, err := BinarySHA256(filepath.Join(dir, "missing")); err == nil { t.Fatal("missing file accepted") }
}
