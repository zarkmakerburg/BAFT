package incidentclass

import (
 "os"
 "path/filepath"
 "testing"
 "time"
)

func TestSnapshotRoundTrip(t *testing.T) {
 path:=filepath.Join(t.TempDir(),"incidents.json")
 if err:=os.WriteFile(path,[]byte("{}"),0600);err!=nil {t.Fatal(err)}
 snap:=Snapshot{Active:map[string]Incident{"a":{InstanceID:"a",Status:NodeOffline,StartedAt:time.Now().UTC()}}}
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
