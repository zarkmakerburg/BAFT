package bcc

import (
 "crypto/ed25519"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "time"

 "github.com/zarkmakerburg/baft/internal/release"
)

// VerifiedReleaseCatalog authenticates all artifacts against an explicitly
// configured root key. A separate, on-disk trust state prevents accepting
// old revocations or silently rolling back a previously accepted release.
type VerifiedReleaseCatalog struct {
 mu sync.Mutex
 dir string
 statePath string
 revocationsPath string
 root ed25519.PublicKey
}

// NewVerifiedReleaseCatalog pins the root key at process startup. Nothing
// supplied by agents or HTTP callers can replace the trust anchor.
func NewVerifiedReleaseCatalog(dir,rootPublicKeyFile,revocationsFile,statePath string) (*VerifiedReleaseCatalog,error) {
 if strings.TrimSpace(dir)==""||strings.TrimSpace(rootPublicKeyFile)==""||strings.TrimSpace(revocationsFile)==""||strings.TrimSpace(statePath)=="" {
  return nil,errors.New("signed catalog needs directory, root public key, and durable trust state path")
 }
 // State must not be inside the release directory, whose contents VerifyDir
 // validates as an exact signed artifact set.
 absDir,err:=filepath.Abs(dir);if err!=nil{return nil,err}
 absState,err:=filepath.Abs(statePath);if err!=nil{return nil,err}
 rel,err:=filepath.Rel(absDir,absState);if err!=nil{return nil,err}
 if rel=="." || (rel!=".." && !strings.HasPrefix(rel,".."+string(os.PathSeparator))) {
  return nil,errors.New("trust state must be outside signed release directory")
 }
 revAbs,err:=filepath.Abs(revocationsFile);if err!=nil{return nil,err}
 revRel,err:=filepath.Rel(absDir,revAbs);if err!=nil{return nil,err}
 if revRel=="." || (revRel!=".." && !strings.HasPrefix(revRel,".."+string(os.PathSeparator))) {return nil,errors.New("revocation list must be outside signed release directory")}
 raw,err:=os.ReadFile(rootPublicKeyFile);if err!=nil{return nil,fmt.Errorf("read pinned root: %w",err)}
 pub,err:=release.DecodePublic(strings.TrimSpace(string(raw)));if err!=nil{return nil,fmt.Errorf("pinned root: %w",err)}
 if _,err:=release.ReadState(statePath);err!=nil{return nil,err}
 return &VerifiedReleaseCatalog{dir:absDir,statePath:absState,revocationsPath:revocationsFile,root:pub},nil
}

// Verify takes a mutex over trust-state read/verify/commit, so concurrent
// admin reads cannot reorder state updates. Any error fails closed.
func (c *VerifiedReleaseCatalog) Verify(now time.Time) (release.Verified,error) {
 if c==nil {return release.Verified{},errors.New("release catalog not configured")}
 c.mu.Lock()
 defer c.mu.Unlock()
 prior,err:=release.ReadState(c.statePath);if err!=nil{return release.Verified{},err}
 rev,err:=release.ReadEnvelope(c.revocationsPath);if err!=nil{return release.Verified{},fmt.Errorf("revocations: %w",err)}
 v,err:=release.VerifyDir(release.VerifyInput{Dir:c.dir,Root:c.root,Revocations:&rev,Now:now,State:&prior})
 if err!=nil{return release.Verified{},err}
 if now.IsZero(){now=time.Now().UTC()}
 if err:=release.WriteState(c.statePath,prior.Advance(v,now));err!=nil{return release.Verified{},fmt.Errorf("persist release trust state: %w",err)}
 return v,nil
}
