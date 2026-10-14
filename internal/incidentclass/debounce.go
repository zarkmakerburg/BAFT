package incidentclass

import "time"

// Debouncer suppresses transient changes without claiming that unconfirmed
// evidence represents an incident. It is scoped to one tunnel instance.
type Debouncer struct {
 Candidate Status
 Since time.Time
 Count int
}

// Confirm returns a status only after at least minSamples consecutive
// observations of the same actionable state over minDuration.
// Unknown observations reset the candidate; callers must retain their last
// confirmed state rather than declaring recovery.
func (d *Debouncer) Confirm(result Result, minSamples int, minDuration time.Duration) (Status, bool) {
 if minSamples < 1 || minDuration < 0 || result.ObservedAt.IsZero() {
  return Unknown,false
 }
 switch result.Status {
 case Healthy, Recovered, NodeOffline, NetworkPathFailure, TransportFailure:
 default:
  d.Candidate, d.Since, d.Count = Unknown,time.Time{},0
  return Unknown,false
 }
 if result.Status!=d.Candidate || result.ObservedAt.Before(d.Since) {
  d.Candidate,d.Since,d.Count=result.Status,result.ObservedAt,1
 } else if result.ObservedAt.After(d.Since) {
  d.Count++
 }
 if d.Count>=minSamples && result.ObservedAt.Sub(d.Since)>=minDuration {
  return d.Candidate,true
 }
 return Unknown,false
}
