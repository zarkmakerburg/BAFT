package incidentclass

import (
 "errors"
 "sync"
 "time"
)

// Tracker serializes per-instance lifecycle transitions. A caller can persist
// its Snapshot in the existing BCC transaction; Tracker never probes nodes.
type Tracker struct {
 mu sync.Mutex
 active map[string]Incident
 history []Incident
}

func NewTracker(snapshot Snapshot) (*Tracker,error) {
 if len(snapshot.History)>MaxHistory {return nil,errors.New("history exceeds limit")}
 if len(snapshot.Active)>MaxActive {return nil,errors.New("active incidents exceed limit")}
 for _,incident:=range snapshot.History {
  if incident.InstanceID=="" || incident.StartedAt.IsZero() || incident.LastObservedAt.IsZero() || incident.Observations==0 || incident.RecoveredAt==nil || (incident.Status!=NodeOffline && incident.Status!=NetworkPathFailure && incident.Status!=TransportFailure) || incident.RecoveredAt.Before(incident.StartedAt) || !incident.LastObservedAt.Equal(*incident.RecoveredAt) {
   return nil,errors.New("invalid completed incident")
  }
 }
 t:=&Tracker{active:make(map[string]Incident),history:append([]Incident(nil),snapshot.History...)}
 for id,incident:=range snapshot.Active {
  if id==""||incident.InstanceID!=id||incident.StartedAt.IsZero()||incident.LastObservedAt.Before(incident.StartedAt)||incident.Observations==0||incident.RecoveredAt!=nil || (incident.Status!=NodeOffline && incident.Status!=NetworkPathFailure && incident.Status!=TransportFailure) {
   return nil,errors.New("invalid active incident")
  }
  t.active[id]=incident
 }
 return t,nil
}

// Observe updates only the specified tunnel instance. Unknown evidence and
// non-monotonic timestamps never close an active incident.
func (t *Tracker) Observe(instanceID string, result Result) {
 t.mu.Lock()
 defer t.mu.Unlock()
 var current *Incident
 if existing,ok:=t.active[instanceID];ok {copy:=existing;current=&copy}
 next,completed:=Advance(instanceID,current,result)
 if current==nil && next!=nil && len(t.active)>=MaxActive {return Result{Status:Unknown,Reason:"incident_capacity_exceeded",ObservedAt:result.ObservedAt}}
 if next==nil {delete(t.active,instanceID)} else {t.active[instanceID]=*next}
 if completed!=nil {t.history=AddCompleted(t.history,*completed)}
}

func (t *Tracker) Snapshot() Snapshot {
 t.mu.Lock()
 defer t.mu.Unlock()
 out:=Snapshot{Active:make(map[string]Incident,len(t.active)),History:append([]Incident(nil),t.history...)}
 for id,incident:=range t.active {out.Active[id]=incident}
 return out
}

// LastFailure returns the latest observation for an active instance, not a
// fresh health assessment. The caller must separately check observation age.
func (t *Tracker) LastFailure(instanceID string) (Status,time.Time,bool) {
 t.mu.Lock()
 defer t.mu.Unlock()
 incident,ok:=t.active[instanceID]
 return incident.Status,incident.LastObservedAt,ok
}

// ObserveEvidence classifies a fresh observation using the current instance
// status under one lock, then advances the lifecycle without a read/write race.
// A missing or stale observation cannot close an incident.
func (t *Tracker) ObserveEvidence(instanceID string, now time.Time, maxAge time.Duration, evidence Evidence) Result {
 t.mu.Lock()
 defer t.mu.Unlock()
 previous:=Unknown
 var current *Incident
 if existing,ok:=t.active[instanceID];ok {
  copy:=existing
  current=&copy
  previous=existing.Status
 }
 result:=Classify(now,maxAge,evidence,previous)
 next,completed:=Advance(instanceID,current,result)
 if next==nil {delete(t.active,instanceID)} else {t.active[instanceID]=*next}
 if completed!=nil {t.history=AddCompleted(t.history,*completed)}
 return result
}
