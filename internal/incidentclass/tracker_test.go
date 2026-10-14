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
