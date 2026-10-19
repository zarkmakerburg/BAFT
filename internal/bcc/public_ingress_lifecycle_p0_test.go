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
