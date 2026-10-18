package bcc

import (
 "strings"
 "testing"
 "time"

 "github.com/zarkmakerburg/baft/internal/release"
)

func TestNodeVersionReadModel(t *testing.T) {
 now:=time.Date(2026,10,18,15,0,0,0,time.UTC)
 digest:=strings.Repeat("a",64)
 signed:=release.Verified{Manifest:release.Manifest{Version:"v0.3.4",Artifacts:[]release.Artifact{{Name:"baft-linux-amd64",OS:"linux",Arch:"amd64",SHA256:digest}}}}
 node:=Node{ID:"ex-1",Platform:NodePlatformEvidence{OS:"linux",Arch:"amd64",Confirmed:true,Source:"operator-verified",ConfirmedAt:now.Add(-time.Minute)}}
 rec:=ComponentDigestReceipt{NodeID:"ex-1",ObservedAt:now.Add(-30*time.Second),AcceptedAt:now,Digests:map[string]string{"transport":digest}}
 check:=func(node Node,rec ComponentDigestReceipt, verified *release.Verified,want ComponentTrustState) {
  t.Helper()
  got:=NodeComponentReadModel(node,rec,verified,now,15*time.Minute).Components[0]
  if got.State!=want{t.Fatalf("state=%s want=%s",got.State,want)}
  if want!=ComponentVerified && got.Version!=""{t.Fatalf("unexpected version %q",got.Version)}
 }
 check(node,rec,nil,ComponentUnknown)
 check(node,rec,&signed,ComponentVerified)
 mismatched:=rec
 mismatched.Digests=map[string]string{"transport":strings.Repeat("b",64)}
 check(node,mismatched,&signed,ComponentMismatch)
 stale:=rec
 stale.AcceptedAt=now.Add(-time.Hour)
 check(node,stale,&signed,ComponentUnknown)
 unconfirmed:=node
 unconfirmed.Platform.Source="component-report"
 check(unconfirmed,rec,&signed,ComponentUnknown)
 revoked:=node
 revoked.Revoked=true
 check(revoked,rec,&signed,ComponentUnknown)
}
