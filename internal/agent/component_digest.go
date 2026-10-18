package agent

import (
 "crypto/sha256"
 "encoding/hex"
 "io"
 "os"
)

// BinarySHA256 returns the observed digest of a regular installed binary.
// The digest is not a trusted version until BCC verifies a signed manifest.
func BinarySHA256(path string) (string, error) {
 f, err := os.Open(path)
 if err != nil { return "", err }
 defer f.Close()
 info, err := f.Stat()
 if err != nil { return "", err }
 if !info.Mode().IsRegular() { return "", os.ErrInvalid }
 h := sha256.New()
 if _, err := io.Copy(h, f); err != nil { return "", err }
 return hex.EncodeToString(h.Sum(nil)), nil
}
