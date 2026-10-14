package incidentclass

import "time"

// Incident is a sanitized lifecycle record for a single tunnel instance.
// The caller persists snapshots using its existing durable store.
type Incident struct {
 InstanceID string `json:"instance_id"`
 Status Status `json:"status"`
 StartedAt time.Time `json:"started_at"`
 LastObservedAt time.Time `json:"last_observed_at"`
 RecoveredAt *time.Time `json:"recovered_at,omitempty"`
 Reason string `json:"reason"`
 Observations uint64 `json:"observations"`
}

// Advance returns the new active incident and an optional completed incident.
// Unknown evidence cannot close an incident. The caller should key state by
// instance ID to avoid contaminating unrelated sibling tunnels.
func Advance(instanceID string, active *Incident, observation Result) (*Incident, *Incident) {
 if instanceID == "" || observation.ObservedAt.IsZero() {
  return active, nil
 }
 if active != nil && !observation.ObservedAt.After(active.LastObservedAt) {
  return active, nil
 }
 switch observation.Status {
 case Unknown:
  return active, nil
 case Healthy, Recovered:
  if active == nil { return nil, nil }
  completed := *active
  t := observation.ObservedAt
  completed.RecoveredAt = &t
  completed.LastObservedAt = t
  return nil, &completed
 case NodeOffline, NetworkPathFailure, TransportFailure:
  if active == nil {
   return &Incident{InstanceID:instanceID,Status:observation.Status,StartedAt:observation.ObservedAt,LastObservedAt:observation.ObservedAt,Reason:observation.Reason,Observations:1},nil
  }
  next := *active
  next.LastObservedAt = observation.ObservedAt
  next.Status = observation.Status
  next.Reason = observation.Reason
  next.Observations++
  return &next,nil
 default:
  return active,nil
 }
}
