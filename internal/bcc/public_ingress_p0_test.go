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
  {"invalid zero port","arno","tun-1","127.0.0.1:1444",0},
  {"zero public port","arno","tun-1","127.0.0.1:1444",0},
  {"overflow port","arno","tun-1","127.0.0.1:1444",65536},
  {"non-loopback","arno","tun-1","0.0.0.0:1444",54619},
  {"remote target","arno","tun-1","10.0.0.1:1444",54619},
  {"self loop IPv4","arno","tun-1","127.0.0.1:54619",54619},
  {"self loop IPv6","arno","tun-1","[::1]:54619",54619},
  {"invalid target port","arno","tun-1","127.0.0.1:0",54619},
 } {t.Run(tc.name,func(t *testing.T){if _,err:=PlanPublicIngress(tc.node,tc.id,tc.port,tc.target);err==nil{t.Fatal("expected validation error")}})}
}

func TestPlanPublicIngressCanonicalBoundaries(t *testing.T) {
 tests := []struct {
  name, node, tunnel, target, wantTarget string
  port int
  wantError bool
 }{
  {"minimum public port"," ir-main "," tun-a ","127.0.0.1:2","127.0.0.1:2",1,false},
  {"maximum public port","ir-main","tun-a","127.0.0.1:65534","127.0.0.1:65534",65535,false},
  {"IPv6 loopback","ir-main","tun-a","[::1]:9000","[::1]:9000",2054,false},
  {"expanded IPv6 loopback","ir-main","tun-a","[0:0:0:0:0:0:0:1]:9000","[::1]:9000",2054,false},
  {"leading zero target port","ir-main","tun-a","127.0.0.1:09000","127.0.0.1:9000",2054,false},
  {"negative public port","ir-main","tun-a","127.0.0.1:9000","",-1,true},
  {"negative target port","ir-main","tun-a","127.0.0.1:-1","",2054,true},
  {"overflow target port","ir-main","tun-a","127.0.0.1:65536","",2054,true},
  {"empty target port","ir-main","tun-a","127.0.0.1:","",2054,true},
  {"missing target host","ir-main","tun-a",":9000","",2054,true},
  {"hostname target","ir-main","tun-a","localhost:9000","",2054,true},
  {"IPv6 unspecified","ir-main","tun-a","[::]:9000","",2054,true},
  {"IPv6 remote","ir-main","tun-a","[2001:db8::1]:9000","",2054,true},
  {"unbracketed IPv6","ir-main","tun-a","::1:9000","",2054,true},
  {"blank node"," \t ","tun-a","127.0.0.1:9000","",2054,true},
  {"blank tunnel","ir-main"," \t ","127.0.0.1:9000","",2054,true},
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

func TestPlanPublicIngressRejectsControlCharactersInOwnershipKeys(t *testing.T) {
 tests := []struct {
  name, node, tunnel string
 }{
  {"node newline", "ir-main\nforged-audit-entry", "tun-a"},
  {"node carriage return", "ir-main\rforged", "tun-a"},
  {"node embedded tab", "ir\tmain", "tun-a"},
  {"tunnel newline", "ir-main", "tun-a\nforged"},
  {"tunnel null byte", "ir-main", "tun-a\x00"},
  {"tunnel unicode line separator control", "ir-main", "tun-a\u0085"},
 }
 for _, tt := range tests {
  t.Run(tt.name, func(t *testing.T) {
   if got, err := PlanPublicIngress(tt.node, tt.tunnel, 54619, "127.0.0.1:1444"); err == nil {
    t.Fatalf("control character accepted: %+v", got)
   }
  })
 }
 // Ordinary surrounding whitespace is still normalized for compatibility.
 got, err := PlanPublicIngress(" ir-main ", " tun-a ", 54619, "127.0.0.1:1444")
 if err != nil || got.IRNode != "ir-main" || got.TunnelID != "tun-a" {
  t.Fatalf("surrounding whitespace canonicalization regressed: %+v err=%v", got, err)
 }
}
