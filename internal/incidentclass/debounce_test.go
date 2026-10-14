package incidentclass

import (
 "testing"
 "time"
)

func TestDebouncer(t *testing.T) {
 now:=time.Now().UTC()
 var d Debouncer
 sample:=func(status Status,seconds int) (Status,bool) {
  return d.Confirm(Result{Status:status,ObservedAt:now.Add(time.Duration(seconds)*time.Second)},2,time.Second)
 }
 if _,ok:=sample(NetworkPathFailure,0);ok {t.Fatal("premature failure")}
 if status,ok:=sample(NetworkPathFailure,1);!ok||status!=NetworkPathFailure {t.Fatal("missing confirmed failure")}
 if _,ok:=sample(Unknown,2);ok {t.Fatal("unknown confirmed")}
 if _,ok:=sample(Healthy,3);ok {t.Fatal("premature recovery")}
 if status,ok:=sample(Healthy,4);!ok||status!=Healthy {t.Fatal("missing confirmed recovery")}
}

func TestDebouncerRejectsInvalidPolicy(t *testing.T) {
 var d Debouncer
 if _,ok:=d.Confirm(Result{Status:Healthy,ObservedAt:time.Now()},0,0);ok {t.Fatal("invalid policy accepted")}
 if _,ok:=d.Confirm(Result{Status:Healthy,ObservedAt:time.Now()},1,-time.Second);ok {t.Fatal("negative duration accepted")}
}

func TestDebouncerDoesNotCountReplayedObservation(t *testing.T) {
 now:=time.Now().UTC()
 var d Debouncer
 first:=Result{Status:NetworkPathFailure,ObservedAt:now}
 if _,ok:=d.Confirm(first,2,time.Second);ok {t.Fatal("premature confirmation")}
 if _,ok:=d.Confirm(first,2,time.Second);ok {t.Fatal("replay confirmed failure")}
 if _,ok:=d.Confirm(Result{Status:NetworkPathFailure,ObservedAt:now.Add(-time.Second)},2,time.Second);ok {t.Fatal("out-of-order confirmed failure")}
 status,ok:=d.Confirm(Result{Status:NetworkPathFailure,ObservedAt:now.Add(time.Second)},2,time.Second)
 if !ok||status!=NetworkPathFailure {t.Fatal("valid consecutive observations not confirmed")}
}
