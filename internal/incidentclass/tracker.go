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
 t:=&Tracker{active:make(map[string]Incident),history:append([]Incident(nil),snapshot.History...)}
 for id,incident:=range snapshot.Active {
  if id==""||incident.InstanceID!=id||incident.StartedAt.IsZero()||incident.LastObservedAt.Before(incident.StartedAt)||incident.RecoveredAt!=nil {
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
