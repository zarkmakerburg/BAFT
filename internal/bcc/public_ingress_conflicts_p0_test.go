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

// Review note: future tests should cover whitespace-normalized node ownership.

func TestP0PublicIngressReservationRejectsMalformedExisting(t *testing.T) {
 candidate,err:=PlanPublicIngress("arno","tun-a",54619,"127.0.0.1:1444")
 if err!=nil {t.Fatal(err)}
 malformed:=PublicIngressSpec{IRNode:"arno",TunnelID:"tun-b",PublicPort:0,LocalTarget:"127.0.0.1:1445"}
 if err:=ValidatePublicIngressReservation(candidate,[]PublicIngressSpec{malformed});err==nil {
  t.Fatal("invalid existing reservation must fail closed")
 }
}
func TestP0PublicIngressReservationTrimmedNodeCollision(t *testing.T) {
 candidate,err:=PlanPublicIngress("arno","tun-b",54619,"127.0.0.1:1445")
 if err!=nil {t.Fatal(err)}
 old:=PublicIngressSpec{IRNode:" arno ",TunnelID:"tun-a",PublicPort:54619,LocalTarget:"127.0.0.1:1444"}
 if err:=ValidatePublicIngressReservation(candidate,[]PublicIngressSpec{old});err==nil {
  t.Fatal("whitespace must not bypass port ownership")
 }
}
