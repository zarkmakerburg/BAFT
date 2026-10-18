package bcc

import (
 "strings"
 "crypto/ed25519"
 "fmt"
 "runtime"
 "time"

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

// VerifyAndCompareComponentDigest checks the complete signed release directory
// against a pinned root and revocations before trusting any artifact digest.
// Verification failures fail closed to unknown, never mismatch/verified.
func VerifyAndCompareComponentDigest(observed, artifactName string, input release.VerifyInput) (ComponentTrustState, error) {
 verified, err := release.VerifyDir(input)
 if err != nil { return ComponentUnknown, err }
 return CompareVerifiedComponentDigest(observed, verified, artifactName), nil
}

// Ensure the signing authority is supplied by the caller; never trust a key
// provided inside an agent report or an unsigned manifest.
func VerifyComponentReleaseDir(dir string, root ed25519.PublicKey, revocations release.Envelope, now time.Time) (release.Verified, error) {
 return release.VerifyDir(release.VerifyInput{Dir:dir, Root:root, Revocations:&revocations, Now:now})
}

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

// ComponentArtifactName chooses a release artifact using a validated platform.
// No architecture or filename is trusted from an Agent-provided digest.
func ComponentArtifactName(component, goos, arch string) (string, error) {
 if goos != "linux" || (arch != "amd64" && arch != "arm64") { return "", fmt.Errorf("unsupported release platform") }
 var binary string
 switch component {
 case "transport": binary = "baft"
 case "agent": binary = "baft-agent"
 case "bcc": binary = "baft-bcc"
 default: return "", fmt.Errorf("unsupported component")
 }
 return fmt.Sprintf("%s-%s-%s",binary,goos,arch),nil
}

// LocalComponentArtifactName is for local diagnostics only; remote node
// architecture must come from an authenticated, trusted enrollment record.
func LocalComponentArtifactName(component string) (string,error) {
 return ComponentArtifactName(component,runtime.GOOS,runtime.GOARCH)
}
