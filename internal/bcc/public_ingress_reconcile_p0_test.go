package bcc

import "testing"

func TestP0IngressReconcileDecision(t *testing.T) {
 spec,err:=PlanPublicIngress("arno","tun-a",54619,"127.0.0.1:1444")
 if err!=nil {t.Fatal(err)}
 check:=func(obs *PublicIngressObserved,want PublicIngressReconcileDecision,wantErr bool){
  t.Helper()
  got,err:=DecidePublicIngressReconcile(spec,obs)
  if got!=want || (err!=nil)!=wantErr {t.Fatalf("got %s, %v; want %s error=%v",got,err,want,wantErr)}
 }
 check(nil,PublicIngressNeedsApply,false)
 check(&PublicIngressObserved{Spec:spec,OwnerVerified:true,ListenerReady:true,FirewallReady:true},PublicIngressNoop,false)
 check(&PublicIngressObserved{Spec:spec,OwnerVerified:true,ListenerReady:false,FirewallReady:true},PublicIngressNeedsApply,false)
 check(&PublicIngressObserved{Spec:spec,OwnerVerified:false,ListenerReady:true,FirewallReady:true},PublicIngressBlocked,true)
 mismatch:=spec
 mismatch.LocalTarget="127.0.0.1:1445"
 check(&PublicIngressObserved{Spec:mismatch,OwnerVerified:true},PublicIngressBlocked,true)
 mismatch=spec
 mismatch.PublicPort=2053
 check(&PublicIngressObserved{Spec:mismatch,OwnerVerified:true},PublicIngressBlocked,true)
}
