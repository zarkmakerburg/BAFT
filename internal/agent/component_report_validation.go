package agent

import (
 "errors"
 "time"
)

// ValidateComponentReport checks envelope shape and freshness only.
// Authentication and signed-manifest verification must be performed by BCC.
func ValidateComponentReport(r ComponentReport, now time.Time, maxAge time.Duration) error {
 if r.NodeID == "" || r.ObservedAt.IsZero() || maxAge <= 0 { return errors.New("invalid report metadata") }
 age := now.Sub(r.ObservedAt)
 if age < 0 || age > maxAge { return errors.New("stale or future component report") }
 if len(r.Components) != 3 { return errors.New("expected three component observations") }
 names := []string{"transport", "agent", "bcc"}
 for i, item := range r.Components {
  if item.Component != names[i] { return errors.New("unexpected component ordering") }
  switch item.State {
  case "unknown":
   if item.SHA256 != "" { return errors.New("unknown component has digest") }
  case "observed":
   if len(item.SHA256) != 64 { return errors.New("invalid digest length") }
   for _, c := range item.SHA256 { if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) { return errors.New("invalid digest encoding") } }
  default: return errors.New("invalid component state")
  }
 }
 return nil
}
