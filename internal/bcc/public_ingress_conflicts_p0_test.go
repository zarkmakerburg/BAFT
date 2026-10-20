package bcc

import (
 "fmt"
 "testing"
)

func TestP0PublicIngressReservations(t *testing.T) {
 a, err := PlanPublicIngress("arno", "tun-a", 54619, "127.0.0.1:1444")
 if err != nil { t.Fatal(err) }
 if err := ValidatePublicIngressReservation(a, nil); err != nil { t.Fatal(err) }
 if err := ValidatePublicIngressReservation(a, []PublicIngressSpec{a}); err != nil { t.Fatalf("idempotent reservation: %v", err) }
 b, err := PlanPublicIngress("arno", "tun-b", 54619, "127.0.0.1:1445")
 if err != nil { t.Fatal(err) }
 if err := ValidatePublicIngressReservation(b, []PublicIngressSpec{a}); err == nil { t.Fatal("same IR public port collision must fail") }
 b.PublicPort = 54620
 if err := ValidatePublicIngressReservation(b, []PublicIngressSpec{a}); err != nil { t.Fatalf("distinct port should pass: %v", err) }
 b.IRNode = "other-ir"
 b.PublicPort = 54619
 if err := ValidatePublicIngressReservation(b, []PublicIngressSpec{a}); err != nil { t.Fatalf("different IR should pass: %v", err) }
 changed := a
 changed.LocalTarget = "127.0.0.1:1445"
 if err := ValidatePublicIngressReservation(changed, []PublicIngressSpec{a}); err == nil { t.Fatal("same tunnel cannot silently change owned target") }
}

// Canonical ownership checks are covered by TestP0PublicIngressCanonicalOwnership.

func TestP0PublicIngressCanonicalOwnership(t *testing.T) {
 base, err := PlanPublicIngress("arno", "tun-a", 54619, "127.0.0.1:1444")
 if err != nil { t.Fatal(err) }
 canonical, err := PlanPublicIngress("arno", "tun-a", 54619, "[::1]:1444")
 if err != nil { t.Fatal(err) }
 equivalent := canonical
 equivalent.IRNode = " arno "
 equivalent.TunnelID = " tun-a "
 equivalent.LocalTarget = "[0:0:0:0:0:0:0:1]:1444"
 if err := ValidatePublicIngressReservation(equivalent, []PublicIngressSpec{canonical}); err != nil {
  t.Fatalf("equivalent canonical reservations should be idempotent: %v", err)
 }
 collision := base
 collision.IRNode = " arno "
 collision.TunnelID = "tun-b"
 stored := base
 stored.IRNode = " arno "
 if err := ValidatePublicIngressReservation(collision, []PublicIngressSpec{stored}); err == nil {
  t.Fatal("whitespace in stored IR node must not bypass public port ownership")
 }
 malformed := base
 malformed.LocalTarget = "0.0.0.0:1444"
 if err := ValidatePublicIngressReservation(base, []PublicIngressSpec{malformed}); err == nil {
  t.Fatal("invalid stored reservations must fail closed")
 }
}

func TestP0PublicIngressReservationFailClosedAcrossNodes(t *testing.T) {
 candidate, err := PlanPublicIngress("ir-main", "tun-b", 2053, "127.0.0.1:9001")
 if err != nil { t.Fatal(err) }
 tests := []struct {
  name string
  existing PublicIngressSpec
  wantError bool
 }{
  {"same node and port", PublicIngressSpec{IRNode:"ir-main", TunnelID:"tun-a", PublicPort:2053, LocalTarget:"127.0.0.1:9000"}, true},
  {"trimmed node and port", PublicIngressSpec{IRNode:" ir-main ", TunnelID:"tun-a", PublicPort:2053, LocalTarget:"127.0.0.1:9000"}, true},
  {"different node", PublicIngressSpec{IRNode:"ir-other", TunnelID:"tun-a", PublicPort:2053, LocalTarget:"127.0.0.1:9000"}, false},
  {"different port", PublicIngressSpec{IRNode:"ir-main", TunnelID:"tun-a", PublicPort:2054, LocalTarget:"127.0.0.1:9000"}, false},
  {"invalid stored target", PublicIngressSpec{IRNode:"ir-other", TunnelID:"tun-a", PublicPort:2053, LocalTarget:"0.0.0.0:9000"}, true},
  {"invalid stored port", PublicIngressSpec{IRNode:"ir-other", TunnelID:"tun-a", PublicPort:0, LocalTarget:"127.0.0.1:9000"}, true},
  {"empty stored tunnel", PublicIngressSpec{IRNode:"ir-other", TunnelID:"", PublicPort:2053, LocalTarget:"127.0.0.1:9000"}, true},
 }
 for _, tt := range tests {
  t.Run(tt.name,func(t *testing.T){
   err := ValidatePublicIngressReservation(candidate, []PublicIngressSpec{tt.existing})
   if (err != nil) != tt.wantError { t.Fatalf("error=%v, wantError=%t",err,tt.wantError) }
  })
 }
}

func TestP0PublicIngressRejectsMalformedReservationAfterMatchingNode(t *testing.T) {
 candidate, err := PlanPublicIngress("ir-main", "tun-a", 2053, "127.0.0.1:9000")
 if err != nil { t.Fatal(err) }
 existing := []PublicIngressSpec{
  candidate,
  {IRNode:"ir-other",TunnelID:"tun-b",PublicPort:0,LocalTarget:"127.0.0.1:9001"},
 }
 if err := ValidatePublicIngressReservation(candidate, existing); err == nil {
  t.Fatal("an idempotent match must not conceal later malformed reservations")
 }
}

func TestP0PublicIngressRejectsConflictingStoredSet(t *testing.T) {
 candidate, err := PlanPublicIngress("ir-main", "new-tunnel", 2055, "127.0.0.1:9005")
 if err != nil { t.Fatal(err) }
 tests := []struct{
  name string
  existing []PublicIngressSpec
  wantError bool
 }{
  {"different tunnels same node same port", []PublicIngressSpec{
   {IRNode:"ir-main",TunnelID:"tun-a",PublicPort:2053,LocalTarget:"127.0.0.1:9000"},
   {IRNode:"ir-main",TunnelID:"tun-b",PublicPort:2053,LocalTarget:"127.0.0.1:9001"},
  },true},
  {"same tunnel conflicting target", []PublicIngressSpec{
   {IRNode:"ir-main",TunnelID:"tun-a",PublicPort:2053,LocalTarget:"127.0.0.1:9000"},
   {IRNode:"ir-main",TunnelID:"tun-a",PublicPort:2053,LocalTarget:"127.0.0.1:9001"},
  },true},
  {"same tunnel conflicting port", []PublicIngressSpec{
   {IRNode:"ir-main",TunnelID:"tun-a",PublicPort:2053,LocalTarget:"127.0.0.1:9000"},
   {IRNode:"ir-main",TunnelID:"tun-a",PublicPort:2054,LocalTarget:"127.0.0.1:9000"},
  },true},
  {"conflict on other node", []PublicIngressSpec{
   {IRNode:"ir-other",TunnelID:"tun-a",PublicPort:2053,LocalTarget:"127.0.0.1:9000"},
   {IRNode:"ir-other",TunnelID:"tun-b",PublicPort:2053,LocalTarget:"127.0.0.1:9001"},
  },true},
  {"identical duplicate is idempotent", []PublicIngressSpec{
   {IRNode:"ir-main",TunnelID:"tun-a",PublicPort:2053,LocalTarget:"127.0.0.1:9000"},
   {IRNode:" ir-main ",TunnelID:" tun-a ",PublicPort:2053,LocalTarget:"127.0.0.1:9000"},
  },false},
  {"same port on different nodes", []PublicIngressSpec{
   {IRNode:"ir-main",TunnelID:"tun-a",PublicPort:2053,LocalTarget:"127.0.0.1:9000"},
   {IRNode:"ir-other",TunnelID:"tun-b",PublicPort:2053,LocalTarget:"127.0.0.1:9001"},
  },false},
 }
 for _,tt := range tests {
  t.Run(tt.name,func(t *testing.T){
   err := ValidatePublicIngressReservation(candidate,tt.existing)
   if (err!=nil)!=tt.wantError {t.Fatalf("error=%v wantError=%t",err,tt.wantError)}
  })
 }
}

func TestP0PublicIngressLargeReservationSet(t *testing.T) {
 const count = 2000
 existing := make([]PublicIngressSpec, 0, count)
 for i := 0; i < count; i++ {
  spec, err := PlanPublicIngress("ir-main", fmt.Sprintf("tunnel-%d",i), 10000+i, "127.0.0.1:9000")
  if err != nil {t.Fatal(err)}
  existing=append(existing,spec)
 }
 candidate,err:=PlanPublicIngress("ir-main","new-tunnel",20000,"127.0.0.1:9001")
 if err!=nil {t.Fatal(err)}
 if err:=ValidatePublicIngressReservation(candidate,existing);err!=nil {t.Fatalf("valid large set rejected: %v",err)}
 candidate.PublicPort=10000+count-1
 if err:=ValidatePublicIngressReservation(candidate,existing);err==nil {t.Fatal("collision at end of large set must fail")}
 existing[count-1].PublicPort=existing[0].PublicPort
 candidate.PublicPort=20000
 if err:=ValidatePublicIngressReservation(candidate,existing);err==nil {t.Fatal("corrupt large set must fail closed")}
}

func TestP0PublicIngressOwnershipMatrix(t *testing.T) {
 base := PublicIngressSpec{IRNode:"ir-a",TunnelID:"tun-a",PublicPort:2053,LocalTarget:"127.0.0.1:9000"}
 candidate := base
 // Each entry exercises an independent allocator decision, not an agent apply.
 tests := []struct {
  name string
  candidate PublicIngressSpec
  existing []PublicIngressSpec
  wantError bool
 }{
  {"01 empty set",base,nil,false},
  {"02 exact replay",base,[]PublicIngressSpec{base},false},
  {"03 whitespace replay",base,[]PublicIngressSpec{{IRNode:" ir-a ",TunnelID:" tun-a ",PublicPort:2053,LocalTarget:"127.0.0.1:09000"}},false},
  {"04 IPv4 mapped replay",base,[]PublicIngressSpec{{IRNode:"ir-a",TunnelID:"tun-a",PublicPort:2053,LocalTarget:"[::ffff:127.0.0.1]:9000"}},false},
  {"05 same tunnel different port",base,[]PublicIngressSpec{{IRNode:"ir-a",TunnelID:"tun-a",PublicPort:2054,LocalTarget:"127.0.0.1:9000"}},true},
  {"06 same tunnel different target",base,[]PublicIngressSpec{{IRNode:"ir-a",TunnelID:"tun-a",PublicPort:2053,LocalTarget:"127.0.0.1:9001"}},true},
  {"07 different tunnel same port",base,[]PublicIngressSpec{{IRNode:"ir-a",TunnelID:"tun-b",PublicPort:2053,LocalTarget:"127.0.0.1:9000"}},true},
  {"08 different tunnel different port",base,[]PublicIngressSpec{{IRNode:"ir-a",TunnelID:"tun-b",PublicPort:2054,LocalTarget:"127.0.0.1:9000"}},false},
  {"09 other node same port",base,[]PublicIngressSpec{{IRNode:"ir-b",TunnelID:"tun-b",PublicPort:2053,LocalTarget:"127.0.0.1:9000"}},false},
  {"10 other node same tunnel",base,[]PublicIngressSpec{{IRNode:"ir-b",TunnelID:"tun-a",PublicPort:2053,LocalTarget:"127.0.0.1:9000"}},false},
  {"11 malformed stored node",base,[]PublicIngressSpec{{IRNode:" ",TunnelID:"tun-b",PublicPort:2054,LocalTarget:"127.0.0.1:9000"}},true},
  {"12 malformed stored tunnel",base,[]PublicIngressSpec{{IRNode:"ir-b",TunnelID:" ",PublicPort:2054,LocalTarget:"127.0.0.1:9000"}},true},
  {"13 malformed stored public port",base,[]PublicIngressSpec{{IRNode:"ir-b",TunnelID:"tun-b",PublicPort:65536,LocalTarget:"127.0.0.1:9000"}},true},
  {"14 malformed stored local target",base,[]PublicIngressSpec{{IRNode:"ir-b",TunnelID:"tun-b",PublicPort:2054,LocalTarget:"10.0.0.1:9000"}},true},
  {"15 conflicting stored ports",base,[]PublicIngressSpec{{IRNode:"ir-b",TunnelID:"tun-b",PublicPort:2054,LocalTarget:"127.0.0.1:9000"},{IRNode:"ir-b",TunnelID:"tun-c",PublicPort:2054,LocalTarget:"127.0.0.1:9001"}},true},
  {"16 conflicting stored tunnel targets",base,[]PublicIngressSpec{{IRNode:"ir-b",TunnelID:"tun-b",PublicPort:2054,LocalTarget:"127.0.0.1:9000"},{IRNode:"ir-b",TunnelID:"tun-b",PublicPort:2054,LocalTarget:"127.0.0.1:9001"}},true},
  {"17 conflicting stored tunnel ports",base,[]PublicIngressSpec{{IRNode:"ir-b",TunnelID:"tun-b",PublicPort:2054,LocalTarget:"127.0.0.1:9000"},{IRNode:"ir-b",TunnelID:"tun-b",PublicPort:2055,LocalTarget:"127.0.0.1:9000"}},true},
  {"18 canonical duplicate stored records",base,[]PublicIngressSpec{{IRNode:"ir-b",TunnelID:"tun-b",PublicPort:2054,LocalTarget:"127.0.0.1:9000"},{IRNode:" ir-b ",TunnelID:" tun-b ",PublicPort:2054,LocalTarget:"127.0.0.1:09000"}},false},
  {"19 invalid record after exact replay",base,[]PublicIngressSpec{base,{IRNode:"ir-b",TunnelID:"tun-b",PublicPort:0,LocalTarget:"127.0.0.1:9000"}},true},
  {"20 candidate invalid despite valid stored",PublicIngressSpec{IRNode:"ir-a",TunnelID:"tun-c",PublicPort:0,LocalTarget:"127.0.0.1:9000"},[]PublicIngressSpec{base},true},
 }
 _ = candidate
 for _,tt := range tests {
  t.Run(tt.name,func(t *testing.T){
   err:=ValidatePublicIngressReservation(tt.candidate,tt.existing)
   if (err!=nil)!=tt.wantError {t.Fatalf("error=%v wantError=%t",err,tt.wantError)}
  })
 }
}
