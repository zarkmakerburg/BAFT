package bcc

import (
 "crypto/sha256"
 "encoding/base64"
 "encoding/hex"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "github.com/zarkmakerburg/baft/internal/release"
)

func TestVerifyAndCompareComponentDigestTamperedManifest(t *testing.T) {
 dir := t.TempDir()
 now := time.Date(2026,10,18,12,0,0,0,time.UTC)
 rootPub,rootKey,err := release.GenerateKey();if err!=nil{t.Fatal(err)}
 relPub,relKey,err := release.GenerateKey();if err!=nil{t.Fatal(err)}
 cert,err:=release.Certify(rootKey,relPub,now.Add(-time.Hour),now.Add(24*time.Hour));if err!=nil{t.Fatal(err)}
 rev,err:=release.SignRevocations(rootKey,1,nil,now.Add(-time.Hour),now.Add(24*time.Hour));if err!=nil{t.Fatal(err)}
 name:="baft-linux-amd64"
 binary:=[]byte("test-baft-binary")
 if err:=os.WriteFile(filepath.Join(dir,name),binary,0755);err!=nil{t.Fatal(err)}
 _,err=release.SignDir(release.SignInput{Dir:dir,Version:"v1.2.3",Commit:strings.Repeat("a",40),CreatedAt:now,Provenance:release.Provenance{Builder:"test",GoVersion:"go1.27"},Key:relKey,Cert:cert,Root:rootPub});if err!=nil{t.Fatal(err)}
 digest:=sha256.Sum256(binary)
 input:=release.VerifyInput{Dir:dir,Root:rootPub,Revocations:&rev,Now:now}
 state,err:=VerifyAndCompareComponentDigest(hex.EncodeToString(digest[:]),name,input)
 if err!=nil||state!=ComponentVerified{t.Fatalf("verified: %s %v",state,err)}
 manifestPath:=filepath.Join(dir,release.ManifestFile)
 env,err:=release.ReadEnvelope(manifestPath);if err!=nil{t.Fatal(err)}
 payload,err:=base64.StdEncoding.DecodeString(env.Payload);if err!=nil{t.Fatal(err)}
 env.Payload=base64.StdEncoding.EncodeToString([]byte(strings.Replace(string(payload),"v1.2.3","v9.9.9",1)))
 if err:=release.WriteEnvelope(manifestPath,env);err!=nil{t.Fatal(err)}
 state,err=VerifyAndCompareComponentDigest(hex.EncodeToString(digest[:]),name,input)
 if err==nil||state!=ComponentUnknown{t.Fatalf("tampered manifest accepted: %s %v",state,err)}
}
