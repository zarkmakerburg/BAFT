package bcc

import "testing"

func TestP0PublicIngressLifecycle(t *testing.T) {
	spec, err := PlanPublicIngress("arno", "tun-a", 54619, "127.0.0.1:1444")
	if err != nil {
		t.Fatal(err)
	}
	state := PublicIngressLifecycle{Spec: spec, Status: PublicIngressPending}
	wrong := spec
	wrong.TunnelID = "tun-b"
	if _, err := TransitionPublicIngress(state, wrong, PublicIngressApplied); err == nil {
		t.Fatal("wrong owner must fail")
	}
	if _, err := TransitionPublicIngress(state, spec, PublicIngressPending); err == nil {
		t.Fatal("no-op transition must fail")
	}
	state, err = TransitionPublicIngress(state, spec, PublicIngressApplied)
	if err != nil || !state.Applied {
		t.Fatalf("apply transition: %+v %v", state, err)
	}
	if _, err := TransitionPublicIngress(state, spec, PublicIngressPending); err == nil {
		t.Fatal("applied cannot go directly pending")
	}
	state, err = TransitionPublicIngress(state, spec, PublicIngressRemoved)
	if err != nil || state.Applied {
		t.Fatalf("remove transition: %+v %v", state, err)
	}
	if _, err := TransitionPublicIngress(state, spec, PublicIngressApplied); err == nil {
		t.Fatal("removed cannot be reapplied")
	}
}

func TestP0PublicIngressLifecycleRejectsMalformedResource(t *testing.T) {
	valid, err := PlanPublicIngress("arno", "tun-a", 54619, "127.0.0.1:1444")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*PublicIngressSpec)
	}{
		{"missing node", func(s *PublicIngressSpec) { s.IRNode = " " }},
		{"missing tunnel", func(s *PublicIngressSpec) { s.TunnelID = "" }},
		{"zero public port", func(s *PublicIngressSpec) { s.PublicPort = 0 }},
		{"overflow public port", func(s *PublicIngressSpec) { s.PublicPort = 65536 }},
		{"non loopback target", func(s *PublicIngressSpec) { s.LocalTarget = "192.0.2.1:1444" }},
		{"invalid target ip", func(s *PublicIngressSpec) { s.LocalTarget = "127.999.0.1:1444" }},
		{"zero target port", func(s *PublicIngressSpec) { s.LocalTarget = "127.0.0.1:0" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec := valid
			tc.mutate(&spec)
			for _, next := range []string{PublicIngressApplied, PublicIngressRemoved, PublicIngressFailed} {
				state := PublicIngressLifecycle{Spec: spec, Status: PublicIngressPending}
				got, err := TransitionPublicIngress(state, spec, next)
				if err == nil {
					t.Fatalf("invalid resource transitioned to %s", next)
				}
				if got != state {
					t.Fatalf("rejected transition changed state: %+v", got)
				}
			}
		})
	}
}

func TestP0IngressLifecycleCanonicalOwnershipAndInvariants(t *testing.T) {
 spec,err:=PlanPublicIngress("ir-main","tun-a",2053,"127.0.0.1:9000")
 if err!=nil {t.Fatal(err)}
 owner:=PublicIngressSpec{IRNode:" ir-main ",TunnelID:" tun-a ",PublicPort:2053,LocalTarget:"127.0.0.1:09000"}
 state:=PublicIngressLifecycle{Spec:spec,Status:PublicIngressPending}
 got,err:=TransitionPublicIngress(state,owner,PublicIngressApplied)
 if err!=nil||!got.Applied {t.Fatalf("canonical owner should be accepted: %+v %v",got,err)}
 for _,tt:=range []struct{name string; state PublicIngressLifecycle}{
  {"pending marked applied",PublicIngressLifecycle{Spec:spec,Status:PublicIngressPending,Applied:true}},
  {"applied marked unapplied",PublicIngressLifecycle{Spec:spec,Status:PublicIngressApplied,Applied:false}},
  {"unknown status",PublicIngressLifecycle{Spec:spec,Status:"UNKNOWN",Applied:true}},
 }{
  t.Run(tt.name,func(t *testing.T){
   previous:=tt.state
   result,err:=TransitionPublicIngress(previous,spec,PublicIngressRemoved)
   if err==nil {t.Fatal("invalid lifecycle state accepted")}
   if result!=previous {t.Fatalf("rejected transition changed state: %+v",result)}
  })
 }
 badOwner:=owner
 badOwner.LocalTarget="0.0.0.0:9000"
 if _,err:=TransitionPublicIngress(state,badOwner,PublicIngressApplied);err==nil {t.Fatal("invalid owner accepted")}
}
