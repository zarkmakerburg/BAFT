package bcc

import "testing"

func TestP0IngressRetireProof(t *testing.T) {
 spec,err:=PlanPublicIngress("arno","tun-a",54619,"127.0.0.1:1444")
 if err!=nil {t.Fatal(err)}
 proof:=PublicIngressRetireProof{Spec:spec,Generation:7,OwnerVerified:true}
 if err:=CanRetirePublicIngress(spec,7,proof);err!=nil{t.Fatal(err)}
 if err:=CanRetirePublicIngress(spec,6,proof);err==nil{t.Fatal("stale generation must fail")}
 if err:=CanRetirePublicIngress(spec,0,proof);err==nil{t.Fatal("zero generation must fail")}
 proof.OwnerVerified=false
 if err:=CanRetirePublicIngress(spec,7,proof);err==nil{t.Fatal("unverified ownership must fail")}
 proof.OwnerVerified=true
 proof.Spec.TunnelID="tun-b"
 if err:=CanRetirePublicIngress(spec,7,proof);err==nil{t.Fatal("other tunnel resource must not be retired")}
}
