package incidentclass

import (
 "strings"
 "fmt"
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
 _,err=NewTracker(Snapshot{History:[]Incident{{InstanceID:"a",StartedAt:now,LastObservedAt:end,RecoveredAt:&end,Status:NodeOffline,Observations:1}}})
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

func TestTrackerActiveCapacityDoesNotEvictExistingIncident(t *testing.T) {
 now:=time.Now().UTC()
 tracker,err:=NewTracker(Snapshot{})
 if err!=nil {t.Fatal(err)}
 for i:=0;i<MaxActive;i++ {
  tracker.Observe(fmt.Sprintf("instance-%d",i),Result{Status:NodeOffline,ObservedAt:now})
 }
 if len(tracker.Snapshot().Active)!=MaxActive {t.Fatal("active capacity not reached")}
 tracker.Observe("extra",Result{Status:NodeOffline,ObservedAt:now})
 if len(tracker.Snapshot().Active)!=MaxActive {t.Fatal("capacity exceeded")}
 if _,_,ok:=tracker.LastFailure("instance-0");!ok {t.Fatal("existing incident evicted")}
 tracker.Observe("instance-0",Result{Status:Recovered,ObservedAt:now.Add(time.Second)})
 tracker.Observe("extra",Result{Status:NodeOffline,ObservedAt:now.Add(2*time.Second)})
 if _,_,ok:=tracker.LastFailure("extra");!ok {t.Fatal("capacity not released after recovery")}
}
func TestTrackerRejectsOversizedActiveSnapshot(t *testing.T) {
 now:=time.Now().UTC()
 active:=make(map[string]Incident)
 for i:=0;i<=MaxActive;i++ {
  id:=fmt.Sprintf("instance-%d",i)
  active[id]=Incident{InstanceID:id,Status:NodeOffline,StartedAt:now,LastObservedAt:now,Observations:1}
 }
 if _,err:=NewTracker(Snapshot{Active:active});err==nil {t.Fatal("oversized active snapshot accepted")}
}

func TestObserveEvidenceCapacityLimit(t *testing.T) {
 now:=time.Now().UTC()
 tracker,err:=NewTracker(Snapshot{})
 if err!=nil {t.Fatal(err)}
 for i:=0;i<MaxActive;i++ {
  tracker.Observe(fmt.Sprintf("slot-%d",i),Result{Status:NodeOffline,ObservedAt:now})
 }
 evidence:=Evidence{ObservedAt:now.Add(time.Second),HeartbeatKnown:true,HeartbeatFresh:false}
 result:=tracker.ObserveEvidence("overflow",now.Add(time.Second),time.Minute,evidence)
 if result.Status!=Unknown {t.Fatalf("unexpected result: %+v",result)}
 if len(tracker.Snapshot().Active)!=MaxActive {t.Fatal("overflow changed active count")}
}

func TestNewTrackerRejectsOversizedPersistedFields(t *testing.T) {
 now:=time.Now().UTC()
 valid:=Incident{InstanceID:"node",Status:NodeOffline,StartedAt:now,LastObservedAt:now,Observations:1}
 oversizedID:=valid
 oversizedID.InstanceID=strings.Repeat("i",MaxInstanceIDBytes+1)
 if _,err:=NewTracker(Snapshot{Active:map[string]Incident{oversizedID.InstanceID:oversizedID}});err==nil {t.Fatal("oversized active ID accepted")}
 oversizedReason:=valid
 oversizedReason.Reason=strings.Repeat("r",MaxReasonBytes+1)
 if _,err:=NewTracker(Snapshot{Active:map[string]Incident{"node":oversizedReason}});err==nil {t.Fatal("oversized active reason accepted")}
 completed:=oversizedReason
 completed.RecoveredAt=&now
 if _,err:=NewTracker(Snapshot{History:[]Incident{completed}});err==nil {t.Fatal("oversized completed reason accepted")}
}
