package agent

import (
 "errors"
 "strings"
 "time"
)

// ComponentReport carries local observations. BCC must authenticate the sender,
// enforce freshness and verify each digest against a signed release manifest.
type ComponentReport struct {
 NodeID string `json:"node_id"`
 ObservedAt time.Time `json:"observed_at"`
 Components []ComponentObservation `json:"components"`
}

// BuildComponentReport never substitutes claimed versions for binary evidence.
// A caller must send it through an authenticated transport to BCC.
func BuildComponentReport(nodeID string, now time.Time, paths map[string]string) (ComponentReport, error) {
 if strings.TrimSpace(nodeID) == "" || now.IsZero() { return ComponentReport{}, errors.New("node id and observation time required") }
 return ComponentReport{NodeID: nodeID, ObservedAt: now.UTC(), Components: ObserveComponents(paths)}, nil
}
