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

func TestSnapshotWorstCaseCapacityFitsBudget(t *testing.T) {
 now:=time.Now().UTC()
 active:=make(map[string]Incident,MaxActive)
 for i:=0;i<MaxActive;i++ {
  id:=fmt.Sprintf("%0256d",i)
  active[id]=Incident{InstanceID:id,Status:NetworkPathFailure,StartedAt:now,LastObservedAt:now,Observations:1,Reason:strings.Repeat("r",MaxReasonBytes)}
 }
 history:=make([]Incident,MaxHistory)
 for i:=range history {
  id:=fmt.Sprintf("%0256d",i)
  history[i]=Incident{InstanceID:id,Status:TransportFailure,StartedAt:now,LastObservedAt:now,RecoveredAt:&now,Observations:1,Reason:strings.Repeat("r",MaxReasonBytes)}
 }
 snapshot:=Snapshot{Active:active,History:history}
 if _,err:=NewTracker(snapshot);err!=nil {t.Fatal(err)}
 encoded,err:=json.Marshal(snapshot)
 if err!=nil {t.Fatal(err)}
 if len(encoded)>MaxSnapshotBytes {t.Fatalf("maximum bounded snapshot needs %d bytes; budget %d",len(encoded),MaxSnapshotBytes)}
 path:=filepath.Join(t.TempDir(),"incidents.json")
 if err:=os.WriteFile(path,[]byte("{}"),0600);err!=nil {t.Fatal(err)}
 if err:=SaveSnapshot(path,snapshot);err!=nil {t.Fatal(err)}
 if _,err:=LoadSnapshot(path);err!=nil {t.Fatal(err)}
}

func TestLoadSnapshotRejectsOversizedFile(t *testing.T) {
 path:=filepath.Join(t.TempDir(),"oversized.json")
 f,err:=os.OpenFile(path,os.O_CREATE|os.O_WRONLY|os.O_EXCL,0600)
 if err!=nil {t.Fatal(err)}
 if err:=f.Truncate(int64(MaxSnapshotBytes)+1);err!=nil {f.Close();t.Fatal(err)}
 if err:=f.Close();err!=nil {t.Fatal(err)}
 if _,err:=LoadSnapshot(path);err==nil {t.Fatal("accepted file larger than snapshot byte budget")}
}

func TestSaveSnapshotRefusesDirectoryWithoutModifyingIt(t *testing.T) {
 dir:=t.TempDir()
 if err:=SaveSnapshot(dir,Snapshot{});err==nil {t.Fatal("accepted directory as snapshot target")}
 if _,err:=LoadSnapshot(dir);err==nil {t.Fatal("accepted directory as snapshot source")}
}

func TestLoadSnapshotRejectsActiveCapacityOverflow(t *testing.T) {
 now:=time.Now().UTC()
 active:=make(map[string]Incident,MaxActive+1)
 for i:=0;i<=MaxActive;i++ {
  id:=fmt.Sprintf("instance-%04d",i)
  active[id]=Incident{InstanceID:id,Status:NodeOffline,StartedAt:now,LastObservedAt:now,Observations:1}
 }
 data,err:=json.Marshal(Snapshot{Active:active})
 if err!=nil {t.Fatal(err)}
 if len(data)>MaxSnapshotBytes {t.Fatal("capacity test unexpectedly exceeds byte budget")}
 path:=filepath.Join(t.TempDir(),"over-capacity.json")
 if err:=os.WriteFile(path,data,0600);err!=nil {t.Fatal(err)}
 if _,err:=LoadSnapshot(path);err==nil {t.Fatal("accepted snapshot exceeding active incident capacity")}
}
