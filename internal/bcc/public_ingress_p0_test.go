package bcc

import (
 "strings"
 "testing"
)

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

func TestPlanPublicIngressCanonicalBoundaries(t *testing.T) {
 tests := []struct {
  name, node, tunnel, target, wantTarget string
  port int
  wantError bool
 }{
  {"minimum public port"," ir-main "," tun-a ","127.0.0.1:1","127.0.0.1:1",1,false},
  {"maximum public port","ir-main","tun-a","127.0.0.1:65535","127.0.0.1:65535",65535,false},
  {"IPv6 loopback","ir-main","tun-a","[::1]:9000","[::1]:9000",2053,false},
  {"expanded IPv6 loopback","ir-main","tun-a","[0:0:0:0:0:0:0:1]:9000","[::1]:9000",2053,false},
  {"leading zero target port","ir-main","tun-a","127.0.0.1:09000","127.0.0.1:9000",2053,false},
  {"negative public port","ir-main","tun-a","127.0.0.1:9000","",-1,true},
  {"negative target port","ir-main","tun-a","127.0.0.1:-1","",2053,true},
  {"overflow target port","ir-main","tun-a","127.0.0.1:65536","",2053,true},
  {"empty target port","ir-main","tun-a","127.0.0.1:","",2053,true},
  {"missing target host","ir-main","tun-a",":9000","",2053,true},
  {"hostname target","ir-main","tun-a","localhost:9000","",2053,true},
  {"IPv6 unspecified","ir-main","tun-a","[::]:9000","",2053,true},
  {"IPv6 remote","ir-main","tun-a","[2001:db8::1]:9000","",2053,true},
  {"unbracketed IPv6","ir-main","tun-a","::1:9000","",2053,true},
  {"blank node"," \t ","tun-a","127.0.0.1:9000","",2053,true},
  {"blank tunnel","ir-main"," \t ","127.0.0.1:9000","",2053,true},
 }
 for _,tt:=range tests {
  t.Run(tt.name,func(t *testing.T){
   got,err:=PlanPublicIngress(tt.node,tt.tunnel,tt.port,tt.target)
   if (err!=nil)!=tt.wantError {t.Fatalf("err=%v wantError=%t",err,tt.wantError)}
   if err!=nil{return}
   if got.LocalTarget!=tt.wantTarget {t.Fatalf("target=%q want=%q",got.LocalTarget,tt.wantTarget)}
   if got.IRNode!=strings.TrimSpace(tt.node)||got.TunnelID!=strings.TrimSpace(tt.tunnel){
    t.Fatalf("identifiers not canonicalized: %+v",got)
   }
  })
 }
}
