package incidentclass

import (
 "strings"
 "encoding/json"
 "fmt"
 "os"
 "path/filepath"
 "testing"
 "time"
)

func TestSnapshotRoundTrip(t *testing.T) {
 path:=filepath.Join(t.TempDir(),"incidents.json")
 if err:=os.WriteFile(path,[]byte("{}"),0600);err!=nil {t.Fatal(err)}
 snap:=Snapshot{Active:map[string]Incident{"a":{InstanceID:"a",Status:NodeOffline,StartedAt:time.Now().UTC(),LastObservedAt:time.Now().UTC(),Observations:1}}}
 if err:=SaveSnapshot(path,snap);err!=nil {t.Fatal(err)}
 got,err:=LoadSnapshot(path)
 if err!=nil {t.Fatal(err)}
 if got.Active["a"].Status!=NodeOffline {t.Fatalf("unexpected status: %v",got.Active)}
 info,err:=os.Stat(path);if err!=nil {t.Fatal(err)}
 if info.Mode().Perm()!=0600 {t.Fatalf("permissions: %v",info.Mode())}
}

func TestSnapshotRefusesSymlink(t *testing.T) {
 dir:=t.TempDir()
 target:=filepath.Join(dir,"target")
 if err:=os.WriteFile(target,[]byte("{}"),0600);err!=nil {t.Fatal(err)}
 link:=filepath.Join(dir,"link")
 if err:=os.Symlink(target,link);err!=nil {t.Skip(err)}
 if err:=SaveSnapshot(link,Snapshot{});err==nil {t.Fatal("accepted symlink write")}
 if _,err:=LoadSnapshot(link);err==nil {t.Fatal("accepted symlink read")}
}

func TestSnapshotBoundAndCorruption(t *testing.T) {
 var history []Incident
 for i:=0;i<MaxHistory+10;i++ {history=AddCompleted(history,Incident{InstanceID:"test"})}
 if len(history)!=MaxHistory {t.Fatal("history unbounded")}
 path:=filepath.Join(t.TempDir(),"bad.json")
 if err:=os.WriteFile(path,[]byte("{broken"),0600);err!=nil {t.Fatal(err)}
 if _,err:=LoadSnapshot(path);err==nil {t.Fatal("accepted malformed JSON")}
}

func TestSnapshotRejectsPermissivePermissions(t *testing.T) {
 path:=filepath.Join(t.TempDir(),"public.json")
 if err:=os.WriteFile(path,[]byte("{}"),0644);err!=nil {t.Fatal(err)}
 if err:=SaveSnapshot(path,Snapshot{});err==nil {t.Fatal("accepted permissive target")}
 if _,err:=LoadSnapshot(path);err==nil {t.Fatal("accepted permissive source")}
}

func TestSnapshotRejectsSemanticallyInvalidJSON(t *testing.T) {
 path:=filepath.Join(t.TempDir(),"invalid.json")
 if err:=os.WriteFile(path,[]byte(`{"active":{"one":{"instance_id":"two"}}}`),0600);err!=nil {t.Fatal(err)}
 if _,err:=LoadSnapshot(path);err==nil {t.Fatal("accepted inconsistent incident identity")}
}

func TestSaveSnapshotRejectsInvalidStateWithoutOverwriting(t *testing.T) {
 path:=filepath.Join(t.TempDir(),"state.json")
 original:=[]byte("{}")
 if err:=os.WriteFile(path,original,0600);err!=nil {t.Fatal(err)}
 invalid:=Snapshot{Active:map[string]Incident{"one":{InstanceID:"other"}}}
 if err:=SaveSnapshot(path,invalid);err==nil {t.Fatal("invalid state persisted")}
 got,err:=os.ReadFile(path)
 if err!=nil {t.Fatal(err)}
 if string(got)!=string(original) {t.Fatal("rejected write modified existing snapshot")}
}

func TestSnapshotAtActiveCapacityFitsConfiguredSize(t *testing.T) {
 now:=time.Now().UTC()
 active:=make(map[string]Incident,MaxActive)
 for i:=0;i<MaxActive;i++ {
  id:=fmt.Sprintf("instance-%04d",i)
  active[id]=Incident{InstanceID:id,Status:NodeOffline,StartedAt:now,LastObservedAt:now,Observations:1}
 }
 data,err:=json.Marshal(Snapshot{Active:active})
 if err!=nil {t.Fatal(err)}
 if len(data)>MaxSnapshotBytes {
  t.Fatalf("max active snapshot requires %d bytes but limit is %d",len(data),MaxSnapshotBytes)
 }
}

func TestSnapshotWorstCaseCapacityFailsWithoutReplacingFile(t *testing.T) {
 now:=time.Now().UTC()
 active:=make(map[string]Incident,MaxActive)
 for i:=0;i<MaxActive;i++ {
  id:=fmt.Sprintf("instance-%04d",i)
  active[id]=Incident{InstanceID:id,Status:NodeOffline,StartedAt:now,LastObservedAt:now,Observations:1,Reason:strings.Repeat("r",MaxReasonBytes)}
 }
 snapshot:=Snapshot{Active:active}
 if _,err:=NewTracker(snapshot);err!=nil {t.Fatal(err)}
 encoded,err:=json.Marshal(snapshot)
 if err!=nil {t.Fatal(err)}
 if len(encoded)<=MaxSnapshotBytes {t.Fatalf("expected worst-case snapshot to exceed budget; got %d bytes, budget %d",len(encoded),MaxSnapshotBytes)}
 path:=filepath.Join(t.TempDir(),"incidents.json")
 original:=[]byte("{}")
 if err:=os.WriteFile(path,original,0600);err!=nil {t.Fatal(err)}
 if err:=SaveSnapshot(path,snapshot);err==nil {t.Fatal("oversized snapshot accepted")}
 got,err:=os.ReadFile(path)
 if err!=nil {t.Fatal(err)}
 if string(got)!=string(original) {t.Fatal("oversized snapshot replaced durable file")}
}
