package incidentclass

import (
 "encoding/json"
 "errors"
 "os"
 "path/filepath"
)

// Snapshot is an optional bounded representation of incident history.
// Storage integration must provide per-instance locking and a private directory.
type Snapshot struct {
 Active map[string]Incident `json:"active"`
 History []Incident `json:"history"`
}

const MaxHistory = 256
const MaxSnapshotBytes = 1 << 20

// AddCompleted returns a copy with oldest completed incidents evicted.
func AddCompleted(history []Incident, completed Incident) []Incident {
 next := append(append([]Incident(nil), history...), completed)
 if len(next) > MaxHistory { next = next[len(next)-MaxHistory:] }
 return next
}

// SaveSnapshot atomically replaces a private, pre-existing snapshot file.
// It intentionally refuses symlinks and does not create directories.
// Caller must serialize writers; this is not a replacement for BCC Store transactions.
func SaveSnapshot(path string, snapshot Snapshot) error {
 info, err := os.Lstat(path)
 if err != nil { return err }
 if !info.Mode().IsRegular() { return errors.New("snapshot target must be a regular file") }
 data, err := json.Marshal(snapshot)
 if err != nil { return err }
 if len(data) > MaxSnapshotBytes { return errors.New("snapshot exceeds size limit") }
 dir := filepath.Dir(path)
 tmp, err := os.CreateTemp(dir, ".incident-*.tmp")
 if err != nil { return err }
 tmpPath := tmp.Name()
 defer os.Remove(tmpPath)
 if err = tmp.Chmod(0600); err != nil { tmp.Close(); return err }
 if _,err = tmp.Write(data); err != nil { tmp.Close(); return err }
 if err = tmp.Sync(); err != nil { tmp.Close(); return err }
 if err = tmp.Close(); err != nil { return err }
 if err = os.Rename(tmpPath,path); err != nil { return err }
 d,err := os.Open(dir)
 if err != nil { return err }
 defer d.Close()
 return d.Sync()
}

// LoadSnapshot rejects oversized, malformed and non-regular snapshot files.
func LoadSnapshot(path string) (Snapshot,error) {
 var snapshot Snapshot
 info,err := os.Lstat(path)
 if err != nil { return snapshot,err }
 if !info.Mode().IsRegular() || info.Size()>MaxSnapshotBytes {
  return snapshot,errors.New("invalid snapshot file")
 }
 data,err := os.ReadFile(path)
 if err != nil { return snapshot,err }
 if len(data)>MaxSnapshotBytes { return snapshot,errors.New("snapshot exceeds size limit") }
 err=json.Unmarshal(data,&snapshot)
 return snapshot,err
}
