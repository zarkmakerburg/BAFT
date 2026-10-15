// Package incidentclass provides conservative, side-effect-free classification
// of tunnel health evidence. It does not initiate probes or mutate live nodes.
package incidentclass

import "time"

type Status string

const (
 Unknown Status = "UNKNOWN"
 Healthy Status = "HEALTHY"
 NodeOffline Status = "NODE_OFFLINE"
 NetworkPathFailure Status = "NETWORK_PATH_FAILURE"
 TransportFailure Status = "TRANSPORT_FAILURE"
 Recovered Status = "RECOVERED"
)

type Evidence struct {
 ObservedAt time.Time
 HeartbeatKnown bool
 HeartbeatFresh bool
 ServiceKnown bool
 ServiceRunning bool
 ListenerKnown bool
 ListenerReady bool
 HandshakeKnown bool
 HandshakeOK bool
 DataPathKnown bool
 DataPathOK bool
 // PathImpaired is true only when an independent, scoped network-path
 // observation supports impairment; an EOF alone is insufficient.
 PathImpaired bool
}

type Result struct {
 Status Status
 Reason string
 ObservedAt time.Time
}

func Classify(now time.Time, maxAge time.Duration, e Evidence, previous Status) Result {
 r := Result{Status: Unknown, Reason: "insufficient_evidence", ObservedAt: e.ObservedAt}
 if maxAge <= 0 || e.ObservedAt.IsZero() || e.ObservedAt.After(now) || now.Sub(e.ObservedAt) > maxAge {
  r.Reason = "stale_or_invalid_evidence"
  return r
 }
 if e.HeartbeatKnown && !e.HeartbeatFresh {
  r.Status, r.Reason = NodeOffline, "heartbeat_stale"
  return r
 }
 if !e.HeartbeatKnown || !e.HeartbeatFresh {
  return r
 }
 if e.ServiceKnown && !e.ServiceRunning {
  r.Status, r.Reason = TransportFailure, "service_stopped"
  return r
 }
 if e.ListenerKnown && !e.ListenerReady {
  r.Status, r.Reason = TransportFailure, "listener_unavailable"
  return r
 }
 if e.PathImpaired && e.HandshakeKnown && !e.HandshakeOK {
  r.Status, r.Reason = NetworkPathFailure, "path_evidence_and_handshake_failure"
  return r
 }
 if e.HandshakeKnown && !e.HandshakeOK {
  r.Status, r.Reason = TransportFailure, "handshake_failed_path_unconfirmed"
  return r
 }
 if e.DataPathKnown && !e.DataPathOK {
  r.Status, r.Reason = TransportFailure, "data_path_failed"
  return r
 }
 if e.ServiceKnown && e.ServiceRunning && e.HandshakeKnown && e.HandshakeOK && e.DataPathKnown && e.DataPathOK {
  r.Status, r.Reason = Healthy, "end_to_end_healthy"
  if previous != Healthy && previous != Unknown && previous != Recovered {
   r.Status, r.Reason = Recovered, "end_to_end_recovered"
  }
 }
 return r
}
