package bcc

import "testing"

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
