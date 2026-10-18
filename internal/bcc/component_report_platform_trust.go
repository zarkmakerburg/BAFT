package bcc

import "github.com/zarkmakerburg/baft/internal/release"

// NodePlatformEvidence is populated only from a separately authenticated
// inventory/enrollment workflow, never from the component digest report.
// Until BCC persists and validates such evidence, callers must pass zero
// value and render the version as unknown.
type NodePlatformEvidence struct {
 OS string
 Arch string
 Confirmed bool
 Source string
}

// TrustedNodeArtifactName fails closed when the platform is absent, merely
// self-reported, or otherwise not independently confirmed by an operator-verified inventory workflow.
func TrustedNodeArtifactName(component string, evidence NodePlatformEvidence) (string, bool) {
 if !evidence.Confirmed || evidence.Source != "operator-verified" {
  return "", false
 }
 name, err := ComponentArtifactName(component,evidence.OS,evidence.Arch)
 if err != nil {return "",false}
 return name, true
}

// AttestNodeComponent combines independently confirmed platform evidence
// with complete signed release verification. Missing platform provenance
// always returns unknown without consulting untrusted manifest data.
func AttestNodeComponent(component, observed string, platform NodePlatformEvidence, input release.VerifyInput) (ComponentTrustState, error) {
 artifact, ok := TrustedNodeArtifactName(component,platform)
 if !ok { return ComponentUnknown,nil }
 return VerifyAndCompareComponentDigest(observed,artifact,input)
}
