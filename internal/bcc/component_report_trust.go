package bcc

import (
 "strings"

 "github.com/zarkmakerburg/baft/internal/release"
)

// ComponentTrustState must only be computed from a manifest that the caller
// already authenticated through release.VerifyDir with a pinned root and
// current revocation list. Untrusted JSON must never be passed as verified.
type ComponentTrustState string

const (
 ComponentVerified ComponentTrustState = "verified"
 ComponentMismatch ComponentTrustState = "mismatch"
 ComponentUnknown ComponentTrustState = "unknown"
)

// CompareVerifiedComponentDigest matches an observed binary digest against a
// release.VerifyDir result, never against an Agent-claimed version. If an
// artifact cannot be uniquely identified, fail closed to unknown.
func CompareVerifiedComponentDigest(observed string, verified release.Verified, artifactName string) ComponentTrustState {
 if observed == "" || observed == "unknown" || !isHexSHA256(observed) || artifactName == "" { return ComponentUnknown }
 matches := 0
 expected := ""
 for _, artifact := range verified.Manifest.Artifacts {
  if artifact.Name == artifactName {
   matches++
   expected = artifact.SHA256
  }
 }
 if matches != 1 || !isHexSHA256(expected) { return ComponentUnknown }
 if strings.EqualFold(observed, expected) { return ComponentVerified }
 return ComponentMismatch
}
