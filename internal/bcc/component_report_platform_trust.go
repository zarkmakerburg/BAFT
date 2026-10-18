package bcc

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
