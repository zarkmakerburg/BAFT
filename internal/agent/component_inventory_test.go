package agent

import (
 "crypto/sha256"
 "encoding/hex"
 "os"
 "path/filepath"
 "testing"
)

func TestObserveComponentsUnknownAndObserved(t *testing.T) {
 path := filepath.Join(t.TempDir(), "baft")
 if err := os.WriteFile(path, []byte("binary"), 0600); err != nil { t.Fatal(err) }
 got := ObserveComponents(map[string]string{"transport": path, "bcc": filepath.Join(t.TempDir(), "absent")})
 if len(got) != 3 { t.Fatalf("expected 3 components, got %d", len(got)) }
 sum := sha256.Sum256([]byte("binary"))
 if got[0].Component != "transport" || got[0].State != "observed" || got[0].SHA256 != hex.EncodeToString(sum[:]) { t.Fatalf("unexpected transport observation: %+v", got[0]) }
 for _, item := range got[1:] {
  if item.State != "unknown" || item.SHA256 != "" { t.Fatalf("unknown component claimed digest: %+v", item) }
 }
}
