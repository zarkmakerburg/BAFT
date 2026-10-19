package bcc

import "testing"

func TestPlanPublicIngress(t *testing.T) {
 got,err:=PlanPublicIngress("arno","tun-1",54619,"127.0.0.1:1444")
 if err!=nil {t.Fatal(err)}
 if got.PublicPort!=54619 || got.LocalTarget!="127.0.0.1:1444" || got.IRNode!="arno" {t.Fatalf("unexpected ingress spec: %+v",got)}
 for _,tc:=range []struct{name,node,id,target string; port int}{
  {"missing node","","tun-1","127.0.0.1:1444",54619},
  {"missing tunnel","arno","","127.0.0.1:1444",54619},
  {"zero public port","arno","tun-1","127.0.0.1:1444",0},
  {"overflow port","arno","tun-1","127.0.0.1:1444",65536},
  {"non-loopback","arno","tun-1","0.0.0.0:1444",54619},
  {"remote target","arno","tun-1","10.0.0.1:1444",54619},
  {"invalid target port","arno","tun-1","127.0.0.1:0",54619},
 } {t.Run(tc.name,func(t *testing.T){if _,err:=PlanPublicIngress(tc.node,tc.id,tc.port,tc.target);err==nil{t.Fatal("expected validation error")}})}
}
