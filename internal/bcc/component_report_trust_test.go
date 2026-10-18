package bcc

import (
 "strings"
 "testing"

 "github.com/zarkmakerburg/baft/internal/release"
)

func TestCompareVerifiedComponentDigest(t *testing.T) {
 good := strings.Repeat("a",64)
 other := strings.Repeat("b",64)
 manifest := release.Verified{Manifest:release.Manifest{Artifacts:[]release.Artifact{{Name:"baft-linux-amd64",SHA256:good}}}}
 cases := []struct{name,observed,artifact string; want ComponentTrustState}{
  {"matching",good,"baft-linux-amd64",ComponentVerified},
  {"different",other,"baft-linux-amd64",ComponentMismatch},
  {"unobserved","unknown","baft-linux-amd64",ComponentUnknown},
  {"unlisted",good,"baft-linux-arm64",ComponentUnknown},
  {"invalid","bad","baft-linux-amd64",ComponentUnknown},
 }
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){if got:=CompareVerifiedComponentDigest(tc.observed,manifest,tc.artifact);got!=tc.want{t.Fatalf("got %s want %s",got,tc.want)}})}
 manifest.Manifest.Artifacts=append(manifest.Manifest.Artifacts,release.Artifact{Name:"baft-linux-amd64",SHA256:good})
 if got:=CompareVerifiedComponentDigest(good,manifest,"baft-linux-amd64");got!=ComponentUnknown{t.Fatalf("duplicate artifact should be unknown: %s",got)}
}
