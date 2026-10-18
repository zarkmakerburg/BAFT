package agent

import (
 "crypto/sha256"
 "encoding/hex"
 "io"
 "os"
)

// BinarySHA256 returns the observed digest of a regular installed binary.
// The digest is not a trusted version until BCC verifies a signed manifest.
// Symlinks are rejected rather than silently following an unexpected target.
func BinarySHA256(path string) (string, error) {
 info, err := os.Lstat(path)
 if err != nil { return "", err }
 if !info.Mode().IsRegular() { return "", os.ErrInvalid }
 f, err := os.Open(path)
 if err != nil { return "", err }
 defer f.Close()
 opened, err := f.Stat()
 if err != nil { return "", err }
 if !opened.Mode().IsRegular() || !os.SameFile(info, opened) { return "", os.ErrInvalid }
 h := sha256.New()
 if _, err := io.Copy(h, f); err != nil { return "", err }
 return hex.EncodeToString(h.Sum(nil)), nil
}
