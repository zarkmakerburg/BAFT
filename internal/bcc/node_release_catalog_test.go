package bcc

import (
 "crypto/sha256"
 "encoding/hex"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "github.com/zarkmakerburg/baft/internal/release"
)

func TestVerifiedReleaseCatalogPinsRootAndPersistsState(t *testing.T) {
 base:=t.TempDir()
 dir:=filepath.Join(base,"signed")
 if err:=os.Mkdir(dir,0700);err!=nil{t.Fatal(err)}
 now:=time.Date(2026,10,18,16,0,0,0,time.UTC)
 rootPub,rootPriv,err:=release.GenerateKey();if err!=nil{t.Fatal(err)}
 signPub,signPriv,err:=release.GenerateKey();if err!=nil{t.Fatal(err)}
 cert,err:=release.Certify(rootPriv,signPub,now.Add(-time.Hour),now.Add(24*time.Hour));if err!=nil{t.Fatal(err)}
 rev,err:=release.SignRevocations(rootPriv,2,nil,now.Add(-time.Hour),now.Add(24*time.Hour));if err!=nil{t.Fatal(err)}
 rootPath:=filepath.Join(base,"root.pub")
 revPath:=filepath.Join(base,"revocations.json")
 trustPath:=filepath.Join(base,"trust.json")
 if err:=os.WriteFile(rootPath,[]byte(release.EncodePublic(rootPub)),0600);err!=nil{t.Fatal(err)}
 if err:=release.WriteEnvelope(revPath,rev);err!=nil{t.Fatal(err)}
 binary:=[]byte("signed BAFT transport")
 if err:=os.WriteFile(filepath.Join(dir,"baft-linux-amd64"),binary,0700);err!=nil{t.Fatal(err)}
 _,err=release.SignDir(release.SignInput{Dir:dir,Version:"v1.2.3",Commit:strings.Repeat("a",40),CreatedAt:now,Provenance:release.Provenance{Builder:"test",GoVersion:"go1.27"},Key:signPriv,Cert:cert,Root:rootPub})
 if err!=nil{t.Fatal(err)}
 cat,err:=NewVerifiedReleaseCatalog(dir,rootPath,revPath,trustPath);if err!=nil{t.Fatal(err)}
 got,err:=cat.Verify(now)
 if err!=nil||got.Version!="v1.2.3"{t.Fatalf("signed catalog rejected: %v %+v",err,got)}
 state,err:=release.ReadState(trustPath);if err!=nil{t.Fatal(err)}
 if state.Version!="v1.2.3"||state.RevocationSequence!=2{t.Fatalf("state not persisted %+v",state)}
 digest:=sha256.Sum256(binary)
 if x:=CompareVerifiedComponentDigest(hex.EncodeToString(digest[:]),got,"baft-linux-amd64");x!=ComponentVerified{t.Fatalf("signed artifact status %s",x)}
 // A root-signed but older revocation envelope must never be accepted.
 older,err:=release.SignRevocations(rootPriv,1,nil,now.Add(-time.Hour),now.Add(24*time.Hour));if err!=nil{t.Fatal(err)}
 if err:=release.WriteEnvelope(revPath,older);err!=nil{t.Fatal(err)}
 if _,err:=cat.Verify(now);err==nil{t.Fatal("older revocation sequence accepted")}
 if _,err:=NewVerifiedReleaseCatalog(dir,rootPath,filepath.Join(dir,"revocations.json"),trustPath);err==nil{t.Fatal("revocations inside signed artifacts accepted")}
 if _,err:=NewVerifiedReleaseCatalog(dir,rootPath,revPath,filepath.Join(dir,"trust.json"));err==nil{t.Fatal("trust state inside signed artifacts accepted")}
}

func TestVerifiedReleaseCatalogRequiresValidPinnedRoot(t *testing.T) {
 base:=t.TempDir()
 rootPath:=filepath.Join(base,"root.pub")
 if err:=os.WriteFile(rootPath,[]byte("not-a-key"),0600);err!=nil{t.Fatal(err)}
 if _,err:=NewVerifiedReleaseCatalog(filepath.Join(base,"signed"),rootPath,filepath.Join(base,"rev.json"),filepath.Join(base,"trust.json"));err==nil{t.Fatal("invalid pinned root accepted")}
}
