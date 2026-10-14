package incidentclass

import (
 "sync"
 "testing"
 "time"
)

func TestTrackerSiblingIsolation(t *testing.T) {
 tracker,err:=NewTracker(Snapshot{})
 if err!=nil {t.Fatal(err)}
 now:=time.Now().UTC()
 var wg sync.WaitGroup
 for _,id:=range []string{"iran-germany","iran-uk"} {
  wg.Add(1)
  go func(id string) {defer wg.Done();tracker.Observe(id,Result{Status:NetworkPathFailure,ObservedAt:now})}(id)
 }
 wg.Wait()
 tracker.Observe("iran-germany",Result{Status:Recovered,ObservedAt:now.Add(time.Minute)})
 snap:=tracker.Snapshot()
 if len(snap.Active)!=1||snap.Active["iran-uk"].InstanceID!="iran-uk"||len(snap.History)!=1||snap.History[0].InstanceID!="iran-germany" {t.Fatalf("sibling contamination: %+v",snap)}
 restored,err:=NewTracker(snap)
 if err!=nil {t.Fatal(err)}
 if _,_,ok:=restored.LastFailure("iran-uk");!ok {t.Fatal("active incident lost after restore")}
}

func TestTrackerRejectsCorruptSnapshot(t *testing.T) {
 _,err:=NewTracker(Snapshot{Active:map[string]Incident{"a":{InstanceID:"other"}}})
 if err==nil {t.Fatal("accepted mismatched incident identity")}
}

func TestTrackerRejectsInvalidCompletedHistory(t *testing.T) {
 now:=time.Now().UTC()
 _,err:=NewTracker(Snapshot{History:[]Incident{{InstanceID:"a",StartedAt:now,LastObservedAt:now}}})
 if err==nil {t.Fatal("accepted unfinished incident as completed history")}
 end:=now.Add(time.Minute)
 _,err=NewTracker(Snapshot{History:[]Incident{{InstanceID:"a",StartedAt:now,LastObservedAt:end,RecoveredAt:&end}}})
 if err!=nil {t.Fatalf("rejected valid recovered incident: %v",err)}
}

func TestObserveEvidenceKeepsFailureUntilFreshRecovery(t *testing.T) {
 tracker,err:=NewTracker(Snapshot{})
 if err!=nil {t.Fatal(err)}
 now:=time.Now().UTC()
 bad:=Evidence{ObservedAt:now,HeartbeatKnown:true,HeartbeatFresh:true,ServiceKnown:true,ServiceRunning:true,HandshakeKnown:true,HandshakeOK:false,PathImpaired:true}
 result:=tracker.ObserveEvidence("one",now,5*time.Minute,bad)
 if result.Status!=NetworkPathFailure {t.Fatal(result)}
 good:=Evidence{ObservedAt:now.Add(time.Minute),HeartbeatKnown:true,HeartbeatFresh:true,ServiceKnown:true,ServiceRunning:true,HandshakeKnown:true,HandshakeOK:true,DataPathKnown:true,DataPathOK:true}
 stale:=good;stale.ObservedAt=now.Add(-time.Hour)
 result=tracker.ObserveEvidence("one",now.Add(time.Minute),5*time.Minute,stale)
 if result.Status!=Unknown {t.Fatal("stale evidence was accepted")}
 if _,_,ok:=tracker.LastFailure("one");!ok {t.Fatal("stale evidence closed incident")}
 result=tracker.ObserveEvidence("one",now.Add(time.Minute),5*time.Minute,good)
 if result.Status!=Recovered {t.Fatal(result)}
 if len(tracker.Snapshot().History)!=1 {t.Fatal("recovery missing")}
}
