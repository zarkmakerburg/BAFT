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

// TODO(P0): verify canonical ownership checks alongside the reservation fix.

func TestP0PublicIngressCanonicalOwnership(t *testing.T) {
 base, err := PlanPublicIngress("arno", "tun-a", 54619, "127.0.0.1:1444")
 if err != nil { t.Fatal(err) }
 equivalent := base
 equivalent.IRNode = " arno "
 equivalent.TunnelID = " tun-a "
 equivalent.LocalTarget = "127.0.0.01:1444"
 // Use canonicalizable IPv6 loopback textual forms instead of ambiguous IPv4 octets.
 equivalent.LocalTarget = "[0:0:0:0:0:0:0:1]:1444"
 canonical, err := PlanPublicIngress("arno", "tun-a", 54619, "[::1]:1444")
 if err != nil { t.Fatal(err) }
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
