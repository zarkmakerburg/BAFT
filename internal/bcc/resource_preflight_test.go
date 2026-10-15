package bcc

import (
	"testing"
	"time"
)

func TestPreflightRejectsCrossPurposeBindCollisionsBeforeJobs(t *testing.T) {
	base := scopedReq("de", "ex-1", "ir-1", 8443, "127.0.0.1:1443", "127.0.0.1:9201", "127.0.0.1:9301")
	for _, tc := range []struct {
		name string
		old  bool
		req  TunnelRequest
	}{
		{"own EX listener and metrics", false, scopedReq("bg", "ex-1", "ir-1", 8443, "127.0.0.1:1444", "127.0.0.1:8443", "127.0.0.1:9302")},
		{"dual-stack EX listener and IPv6 metrics", false, scopedReq("bg", "ex-1", "ir-1", 8443, "127.0.0.1:1444", "[::1]:8443", "127.0.0.1:9302")},
		{"own IR route and metrics", false, scopedReq("bg", "ex-1", "ir-1", 8444, "127.0.0.1:1444", "127.0.0.1:9202", "127.0.0.1:1444")},
		{"EX metrics meets sibling listener", true, scopedReq("bg", "ex-1", "ir-2", 8444, "127.0.0.1:1444", "127.0.0.1:8443", "127.0.0.1:9302")},
		{"EX listener meets sibling metrics", true, scopedReq("bg", "ex-1", "ir-2", 9201, "127.0.0.1:1444", "127.0.0.1:9202", "127.0.0.1:9302")},
		{"IR metrics meets sibling route", true, scopedReq("bg", "ex-2", "ir-1", 8443, "127.0.0.1:1444", "127.0.0.1:9201", "127.0.0.1:1443")},
		{"IR route meets sibling metrics", true, scopedReq("bg", "ex-2", "ir-1", 8443, "127.0.0.1:9301", "127.0.0.1:9201", "127.0.0.1:9302")},
		{"equivalent decimal metrics ports", true, scopedReq("bg", "ex-2", "ir-1", 8443, "127.0.0.1:1444", "127.0.0.1:9201", "127.0.0.1:09301")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := multiInstanceStore(t)
			now := time.Now()
			if tc.old {
				if _, err := s.CreateTunnel(base, now); err != nil {
					t.Fatal(err)
				}
			}
			tunnels, jobs := len(s.ListTunnels()), len(s.ListJobs())
			if _, err := s.BuildPlan(tc.req, now); err == nil {
				t.Error("unsafe plan was accepted")
			}
			if _, err := s.CreateTunnel(tc.req, now); err == nil {
				t.Error("unsafe change queued")
			}
			if len(s.ListTunnels()) != tunnels || len(s.ListJobs()) != jobs {
				t.Fatal("rejected preflight changed tunnel or job inventory")
			}
		})
	}
}

func TestPreflightRejectsInvalidNumericEndpointsBeforeJobs(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*TunnelRequest)
	}{
		{"listener range", func(r *TunnelRequest) { r.Port = 65536 }},
		{"route range", func(r *TunnelRequest) { r.RouteListen = "127.0.0.1:65536" }},
		{"metrics range", func(r *TunnelRequest) { r.IRMetricsListen = "127.0.0.1:65536" }},
		{"metrics padded zero", func(r *TunnelRequest) { r.EXMetricsListen = "127.0.0.1:00000" }},
		{"target invalid IP", func(r *TunnelRequest) { r.Target = "999.0.0.1:2443" }},
		{"target range", func(r *TunnelRequest) { r.Target = "127.0.0.1:65536" }},
		{"ignored legacy metrics", func(r *TunnelRequest) { r.InstanceID = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := multiInstanceStore(t)
			r := scopedReq("de", "ex-1", "ir-1", 8443, "127.0.0.1:1443", "127.0.0.1:9201", "127.0.0.1:9301")
			tc.edit(&r)
			if _, err := s.CreateTunnel(r, time.Now()); err == nil {
				t.Error("invalid endpoint queued a change")
			}
			if len(s.ListTunnels()) != 0 || len(s.ListJobs()) != 0 {
				t.Fatal("invalid endpoint changed inventory")
			}
		})
	}
}

func TestPreflightAllowsDistinctAddressesAndIndependentNodes(t *testing.T) {
	for _, r := range []TunnelRequest{
		scopedReq("de", "ex-1", "ir-1", 8443, "127.0.0.1:1443", "[::1]:9201", "127.0.0.2:1443"),
		scopedReq("de", "ex-1", "ir-1", 8443, "127.0.0.1:1443", "127.0.0.1:1443", "127.0.0.1:8443"),
	} {
		s := multiInstanceStore(t)
		if _, err := s.CreateTunnel(r, time.Now()); err != nil {
			t.Fatalf("independent binds were rejected: %v", err)
		}
	}
}
