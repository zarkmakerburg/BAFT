package agent

// ComponentObservation describes local evidence only; the server must compare
// observed hashes against a separately verified, signed release manifest.
type ComponentObservation struct {
 Component string `json:"component"`
 SHA256 string `json:"sha256,omitempty"`
 State string `json:"state"` // observed, unknown
}

// ObserveComponents does not guess a version when a component is absent,
// inaccessible or otherwise cannot be hashed.
func ObserveComponents(paths map[string]string) []ComponentObservation {
 names := []string{"transport", "agent", "bcc"}
 out := make([]ComponentObservation, 0, len(names))
 for _, name := range names {
  item := ComponentObservation{Component: name, State: "unknown"}
  if path := paths[name]; path != "" {
   if digest, err := BinarySHA256(path); err == nil {
    item.SHA256, item.State = digest, "observed"
   }
  }
  out = append(out, item)
 }
 return out
}
