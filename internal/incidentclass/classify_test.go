package incidentclass

import (
 "testing"
 "time"
)

func TestClassify(t *testing.T) {
 now := time.Date(2026, 10, 14, 20, 0, 0, 0, time.UTC)
 good := Evidence{ObservedAt: now, HeartbeatKnown: true, HeartbeatFresh: true, ServiceKnown: true, ServiceRunning: true, ListenerKnown: true, ListenerReady: true, HandshakeKnown: true, HandshakeOK: true, DataPathKnown: true, DataPathOK: true}
 cases := []struct{name string; evidence Evidence; prev Status; want Status}{
  {"healthy",good,Unknown,Healthy},
  {"recovered",good,NetworkPathFailure,Recovered},
  {"offline",func() Evidence {e:=good;e.HeartbeatFresh=false;return e}(),Healthy,NodeOffline},
  {"stopped",func() Evidence {e:=good;e.ServiceRunning=false;return e}(),Healthy,TransportFailure},
  {"listener",func() Evidence {e:=good;e.ListenerReady=false;return e}(),Healthy,TransportFailure},
  {"path",func() Evidence {e:=good;e.HandshakeOK=false;e.PathImpaired=true;return e}(),Healthy,NetworkPathFailure},
  {"eof_without_path_evidence",func() Evidence {e:=good;e.HandshakeOK=false;return e}(),Healthy,TransportFailure},
  {"payload",func() Evidence {e:=good;e.DataPathOK=false;return e}(),Healthy,TransportFailure},
  {"missing_heartbeat",func() Evidence {e:=good;e.HeartbeatKnown=false;return e}(),Healthy,Unknown},
  {"missing_payload",func() Evidence {e:=good;e.DataPathKnown=false;return e}(),Healthy,Unknown},
  {"stale",func() Evidence {e:=good;e.ObservedAt=now.Add(-time.Hour);return e}(),Healthy,Unknown},
  {"future",func() Evidence {e:=good;e.ObservedAt=now.Add(time.Minute);return e}(),Healthy,Unknown},
 }
 for _, tc := range cases {t.Run(tc.name,func(t *testing.T){got:=Classify(now,5*time.Minute,tc.evidence,tc.prev);if got.Status!=tc.want {t.Fatalf("got %s (%s), want %s",got.Status,got.Reason,tc.want)}})}
}
