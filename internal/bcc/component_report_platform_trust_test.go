package bcc

import "testing"

func TestTrustedNodeArtifactNameFailClosed(t *testing.T) {
 cases:=[]struct{name string; evidence NodePlatformEvidence; ok bool}{
  {"missing",NodePlatformEvidence{},false},
  {"unconfirmed",NodePlatformEvidence{OS:"linux",Arch:"amd64",Source:"enrollment"},false},
  {"self-reported",NodePlatformEvidence{OS:"linux",Arch:"amd64",Confirmed:true,Source:"component-report"},false},
  {"missing-source",NodePlatformEvidence{OS:"linux",Arch:"amd64",Confirmed:true},false},
  {"unsupported",NodePlatformEvidence{OS:"windows",Arch:"amd64",Confirmed:true,Source:"enrollment"},false},
  {"confirmed",NodePlatformEvidence{OS:"linux",Arch:"arm64",Confirmed:true,Source:"enrollment"},true},
 }
 for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){name,ok:=TrustedNodeArtifactName("agent",tc.evidence);if ok!=tc.ok{t.Fatalf("ok=%t want %t",ok,tc.ok)};if ok&&name!="baft-agent-linux-arm64"{t.Fatalf("unexpected name %q",name)};if !ok&&name!=""{t.Fatalf("untrusted artifact %q",name)}})}
}
