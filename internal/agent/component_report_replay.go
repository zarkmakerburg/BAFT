package agent

import (
 "errors"
 "time"
)

// AcceptComponentReportTime rejects duplicate or out-of-order observation times.
// BCC must persist lastAccepted across restarts, authenticate the agent token,
// and compare observed digests to the signed release manifest before trusting it.
func AcceptComponentReportTime(report ComponentReport, lastAccepted, now time.Time, maxAge time.Duration) error {
 if err := ValidateComponentReport(report, now, maxAge); err != nil { return err }
 if !lastAccepted.IsZero() && !report.ObservedAt.After(lastAccepted) {
  return errors.New("replayed or out-of-order component report")
 }
 return nil
}
