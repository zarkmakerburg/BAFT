package incidentclass

import (
 "strings"
 "testing"
 "time"
)

func TestAdvanceLifecycle(t *testing.T) {
 start:=time.Date(2026,10,14,20,0,0,0,time.UTC)
 fail:=Result{Status:NetworkPathFailure,Reason:"path_evidence_and_handshake_failure",ObservedAt:start}
 active,done:=Advance("arno-de",nil,fail)
 if active==nil||done!=nil||active.StartedAt!=start {t.Fatal("missing initial incident")}
 active,done=Advance("arno-de",active,Result{Status:Unknown,ObservedAt:start.Add(time.Minute)})
 if done!=nil||active.Observations!=1 {t.Fatal("unknown evidence altered incident")}
 active,done=Advance("arno-de",active,Result{Status:TransportFailure,ObservedAt:start.Add(2*time.Minute)})
 if done!=nil||active.Observations!=2||active.Status!=TransportFailure {t.Fatal("failed to update incident")}
 active,done=Advance("arno-de",active,Result{Status:Recovered,ObservedAt:start.Add(3*time.Minute)})
 if active!=nil||done==nil||done.RecoveredAt==nil||done.RecoveredAt.Sub(done.StartedAt)!=3*time.Minute {t.Fatal("incorrect recovery timeline")}
}

func TestAdvanceRejectsOutOfOrderAndMissingID(t *testing.T) {
 now:=time.Now().UTC()
 active,_:=Advance("one",nil,Result{Status:NodeOffline,ObservedAt:now})
 next,_:=Advance("one",active,Result{Status:Healthy,ObservedAt:now.Add(-time.Second)})
 if next!=active {t.Fatal("out-of-order event closed incident")}
 next,_=Advance("",active,Result{Status:Healthy,ObservedAt:now.Add(time.Second)})
 if next!=active {t.Fatal("missing instance ID closed incident")}
}

func TestAdvanceRejectsOversizedFields(t *testing.T) {
 now:=time.Now().UTC()
 tooLongID:=strings.Repeat("x",MaxInstanceIDBytes+1)
 tooLongReason:=strings.Repeat("r",MaxReasonBytes+1)
 if active,_:=Advance(tooLongID,nil,Result{Status:NodeOffline,ObservedAt:now});active!=nil {t.Fatal("oversized ID accepted")}
 if active,_:=Advance("valid",nil,Result{Status:NodeOffline,ObservedAt:now,Reason:tooLongReason});active!=nil {t.Fatal("oversized reason accepted")}
}
