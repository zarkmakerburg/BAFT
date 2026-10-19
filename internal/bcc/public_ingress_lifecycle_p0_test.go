package bcc

import "testing"

func TestP0PublicIngressLifecycle(t *testing.T) {
 spec,err:=PlanPublicIngress("arno","tun-a",54619,"127.0.0.1:1444")
 if err!=nil {t.Fatal(err)}
 state:=PublicIngressLifecycle{Spec:spec,Status:PublicIngressPending}
 wrong:=spec
 wrong.TunnelID="tun-b"
 if _,err:=TransitionPublicIngress(state,wrong,PublicIngressApplied);err==nil{t.Fatal("wrong owner must fail")}
 if _,err:=TransitionPublicIngress(state,spec,PublicIngressPending);err==nil{t.Fatal("no-op transition must fail")}
 state,err=TransitionPublicIngress(state,spec,PublicIngressApplied)
 if err!=nil || !state.Applied{t.Fatalf("apply transition: %+v %v",state,err)}
 if _,err:=TransitionPublicIngress(state,spec,PublicIngressPending);err==nil{t.Fatal("applied cannot go directly pending")}
 state,err=TransitionPublicIngress(state,spec,PublicIngressRemoved)
 if err!=nil || state.Applied{t.Fatalf("remove transition: %+v %v",state,err)}
 if _,err:=TransitionPublicIngress(state,spec,PublicIngressApplied);err==nil{t.Fatal("removed cannot be reapplied")}
}
