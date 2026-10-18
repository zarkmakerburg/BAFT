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
 absDir,err=filepath.EvalSymlinks(absDir);if err!=nil{return nil,fmt.Errorf("resolve signed release directory: %w",err)}
 absState,err:=filepath.Abs(statePath);if err!=nil{return nil,err}
 stateParent,err:=filepath.EvalSymlinks(filepath.Dir(absState));if err!=nil{return nil,fmt.Errorf("resolve trust state parent: %w",err)}
 absState=filepath.Join(stateParent,filepath.Base(absState))
 if info,statErr:=os.Lstat(absState);statErr==nil {
  if info.Mode()&os.ModeSymlink!=0 {
   absState,err=filepath.EvalSymlinks(absState);if err!=nil{return nil,fmt.Errorf("resolve trust state symlink: %w",err)}
  }
 } else if !os.IsNotExist(statErr) {return nil,fmt.Errorf("inspect trust state: %w",statErr)}
 rel,err:=filepath.Rel(absDir,absState);if err!=nil{return nil,err}
 if rel=="." || (rel!=".." && !strings.HasPrefix(rel,".."+string(os.PathSeparator))) {
  return nil,errors.New("trust state must be outside signed release directory")
 }
 revAbs,err:=filepath.Abs(revocationsFile);if err!=nil{return nil,err}
 revAbs,err=filepath.EvalSymlinks(revAbs);if err!=nil{return nil,fmt.Errorf("resolve revocations: %w",err)}
 revRel,err:=filepath.Rel(absDir,revAbs);if err!=nil{return nil,err}
 if revRel=="." || (revRel!=".." && !strings.HasPrefix(revRel,".."+string(os.PathSeparator))) {return nil,errors.New("revocation list must be outside signed release directory")}
 raw,err:=os.ReadFile(rootPublicKeyFile);if err!=nil{return nil,fmt.Errorf("read pinned root: %w",err)}
 pub,err:=release.DecodePublic(strings.TrimSpace(string(raw)));if err!=nil{return nil,fmt.Errorf("pinned root: %w",err)}
 if _,err:=release.ReadState(statePath);err!=nil{return nil,err}
 return &VerifiedReleaseCatalog{dir:dir,statePath:absState,revocationsPath:revocationsFile,root:pub},nil
}

// Verify takes a mutex over trust-state read/verify/commit, so concurrent
// admin reads cannot reorder state updates. Any error fails closed.
func (c *VerifiedReleaseCatalog) Verify(now time.Time) (release.Verified,error) {
 if c==nil {return release.Verified{},errors.New("release catalog not configured")}
 c.mu.Lock()
 defer c.mu.Unlock()
 // Resolve the operator-controlled current links on every verification.
 // Recheck that the revocation file remains outside the signed directory.
 absDir,err:=filepath.Abs(c.dir);if err!=nil{return release.Verified{},err}
 absDir,err=filepath.EvalSymlinks(absDir);if err!=nil{return release.Verified{},fmt.Errorf("resolve signed release directory: %w",err)}
 revAbs,err:=filepath.Abs(c.revocationsPath);if err!=nil{return release.Verified{},err}
 revAbs,err=filepath.EvalSymlinks(revAbs);if err!=nil{return release.Verified{},fmt.Errorf("resolve revocations: %w",err)}
 revRel,err:=filepath.Rel(absDir,revAbs);if err!=nil{return release.Verified{},err}
 if revRel=="." || (revRel!=".." && !strings.HasPrefix(revRel,".."+string(os.PathSeparator))) {return release.Verified{},errors.New("revocation list must be outside signed release directory")}
 prior,err:=release.ReadState(c.statePath);if err!=nil{return release.Verified{},err}
 rev,err:=release.ReadEnvelope(revAbs);if err!=nil{return release.Verified{},fmt.Errorf("revocations: %w",err)}
 v,err:=release.VerifyDir(release.VerifyInput{Dir:absDir,Root:c.root,Revocations:&rev,Now:now,State:&prior})
 if err!=nil{return release.Verified{},err}
 if now.IsZero(){now=time.Now().UTC()}
 // A read-only catalog observation must not accept a new release version.
 // Only the independently authenticated revocation sequence may advance.
 if v.Revocations.Sequence>prior.RevocationSequence {
  prior.RevocationSequence=v.Revocations.Sequence
  prior.UpdatedAt=now.UTC().Truncate(time.Second)
  if err:=release.WriteState(c.statePath,prior);err!=nil{return release.Verified{},fmt.Errorf("persist revocation trust state: %w",err)}
 }
 return v,nil
}

 
// AcceptRelease explicitly ratchets the authenticated release after an operator
// has approved the exact version and commit. Downgrades are never implicit.
// Callers must enforce administrator authentication and durable audit logging.
func (c *VerifiedReleaseCatalog) AcceptRelease(now time.Time, expectedVersion, expectedCommit string, allowDowngrade bool) (release.Verified,error) {
 return c.AcceptReleaseWithIntent(now,expectedVersion,expectedCommit,allowDowngrade,nil)
}

// AcceptReleaseWithIntent records a durable operator intent after verification
// and before advancing trust state. The callback must fail closed on errors.
func (c *VerifiedReleaseCatalog) AcceptReleaseWithIntent(now time.Time, expectedVersion, expectedCommit string, allowDowngrade bool, recordIntent func(release.Verified) error) (release.Verified,error) {
 if c==nil{return release.Verified{},errors.New("release catalog not configured")}
 if strings.TrimSpace(expectedVersion)==""||strings.TrimSpace(expectedCommit)==""{return release.Verified{},errors.New("expected version and commit required")}
 c.mu.Lock()
 defer c.mu.Unlock()
 absDir,err:=filepath.Abs(c.dir);if err!=nil{return release.Verified{},err}
 absDir,err=filepath.EvalSymlinks(absDir);if err!=nil{return release.Verified{},err}
 revAbs,err:=filepath.Abs(c.revocationsPath);if err!=nil{return release.Verified{},err}
 revAbs,err=filepath.EvalSymlinks(revAbs);if err!=nil{return release.Verified{},err}
 rel,err:=filepath.Rel(absDir,revAbs);if err!=nil{return release.Verified{},err}
 if rel=="."||(rel!=".."&&!strings.HasPrefix(rel,".."+string(os.PathSeparator))){return release.Verified{},errors.New("revocation list must be outside signed release directory")}
 prior,err:=release.ReadState(c.statePath);if err!=nil{return release.Verified{},err}
 rev,err:=release.ReadEnvelope(revAbs);if err!=nil{return release.Verified{},err}
 v,err:=release.VerifyDir(release.VerifyInput{Dir:absDir,Root:c.root,Revocations:&rev,Now:now,State:&prior,AllowDowngrade:allowDowngrade})
 if err!=nil{return release.Verified{},err}
 if v.Version!=expectedVersion||v.Commit!=expectedCommit{return release.Verified{},errors.New("signed release does not match explicitly approved version and commit")}
 if now.IsZero(){now=time.Now().UTC()}
 if recordIntent!=nil {if err:=recordIntent(v);err!=nil{return release.Verified{},fmt.Errorf("record acceptance intent: %w",err)}}
 if err:=release.WriteState(c.statePath,prior.Advance(v,now));err!=nil{return release.Verified{},fmt.Errorf("persist accepted release trust state: %w",err)}
 return v,nil
}
