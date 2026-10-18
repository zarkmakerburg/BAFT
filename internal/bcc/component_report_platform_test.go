package bcc

import "testing"

func TestComponentArtifactName(t *testing.T) {
 cases:=[]struct{component,os,arch,want string; invalid bool}{
  {"transport","linux","amd64","baft-linux-amd64",false},
  {"agent","linux","arm64","baft-agent-linux-arm64",false},
  {"bcc","linux","amd64","baft-bcc-linux-amd64",false},
  {"xray","linux","amd64","",true},
  {"agent","windows","amd64","",true},
  {"agent","linux","386","",true},
 }
 for _,tc:=range cases {
  got,err:=ComponentArtifactName(tc.component,tc.os,tc.arch)
  if tc.invalid {if err==nil{t.Fatalf("expected rejection for %q/%q/%q",tc.component,tc.os,tc.arch)};continue}
  if err!=nil||got!=tc.want{t.Fatalf("mapping %q/%q/%q: %q %v",tc.component,tc.os,tc.arch,got,err)}
 }
}
