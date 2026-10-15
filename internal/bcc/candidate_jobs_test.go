package bcc

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/tunnelnode"
)

func TestCandidateJobsStageEXBeforeAnyCommit(t *testing.T) {
	s := tunnelStore(t)
	now := time.Now().UTC()
	tn := newTunnel(t, s, now)
	for _, step := range []struct{ node, output string }{{"ex-1", testPairCode}, {"ir-1", testReplyCode}} {
		j := pullAll(t, s, step.node)[0]
		if err := s.AckJobOutput(step.node, "tok-"+step.node, j.ID, "succeeded", "prepared", step.output); err != nil {
			t.Fatal(err)
		}
		if _, err := s.AdvanceTunnels(now); err != nil {
			t.Fatal(err)
		}
	}
	j := pullAll(t, s, "ex-1")[0]
	if j.Type != "tunnel_stage_ex" || j.Params["reply"] != testReplyCode {
		t.Fatalf("configuration must complete in a separate EX staging job before any commit: %+v", j)
	}
	for _, id := range s.st.Tunnels[tn.ID].Jobs {
		if j := s.st.Jobs[id]; j.Type == JobTunnelCommitEX || j.Type == JobTunnelCommitIR {
			t.Fatalf("queued activation before both candidate reports: %+v", j)
		}
	}
}

func goodCandidate(tn Tunnel, node string, now time.Time) tunnelnode.CandidateReport {
	r := tunnelnode.CandidateReport{Version: 1, TunnelID: tn.ID, InstanceID: tn.InstanceID, Role: tunnelnode.RoleEX, Generation: tn.ExpectedGen[node].From, ConfigSHA256: strings.Repeat("a", 64), Resources: true, ObservedAt: now, RouteID: tn.RouteID, MetricsListen: tn.EXMetricsListen, RecordShaping: tn.RecordShaping}
	if node == tn.EXNode {
		r.Listen, r.Target = fmt.Sprintf("0.0.0.0:%d", tn.Port), tn.Target
	} else {
		r.Role, r.PeerAddress, r.RouteListen = tunnelnode.RoleIR, fmt.Sprintf("%s:%d", tn.PublicAddress, tn.Port), tn.RouteListen
		r.MetricsListen = tn.IRMetricsListen
	}
	if r.MetricsListen == "" {
		r.MetricsListen = "127.0.0.1:9191"
	}
	return r
}

func completeCandidateStages(t *testing.T, s *Store, id string, now time.Time) {
	t.Helper()
	for _, node := range []string{"ex-1", "ir-1"} {
		tn, _ := s.GetTunnel(id)
		if !tn.CandidatePreparation {
			return
		}
		j := pullAll(t, s, node)
		if len(j) != 1 || (j[0].Type != JobTunnelStageEX && j[0].Type != JobTunnelValidateCandidate) {
			t.Fatalf("candidate job: %+v", j)
		}
		b, _ := json.Marshal(goodCandidate(tn, node, now))
		if err := s.AckJobOutput(node, "tok-"+node, j[0].ID, "succeeded", string(b), ""); err != nil {
			t.Fatal(err)
		}
		if _, err := s.AdvanceTunnels(now); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCandidateReportsRefuseUnscopedOrStaleSuccess(t *testing.T) {
	for _, kind := range []string{"plain-success", "wrong-tunnel", "wrong-instance", "wrong-role", "missing-digest", "stale", "future", "wrong-route", "wrong-metrics", "no-resources", "wrong-generation", "trailing-json", "unknown-field"} {
		t.Run(kind, func(t *testing.T) {
			s := tunnelStore(t)
			now := time.Now().UTC()
			tn := newTunnel(t, s, now)
			tn.ExpectedGen[tn.EXNode] = GenExpect{From: 3, To: 4}
			s.st.Tunnels[tn.ID] = tn
			for _, step := range []struct{ node, out string }{{tn.EXNode, testPairCode}, {tn.IRNode, testReplyCode}} {
				j := pullAll(t, s, step.node)[0]
				s.AckJobOutput(step.node, "tok-"+step.node, j.ID, "succeeded", "ok", step.out)
				s.AdvanceTunnels(now)
			}
			r := goodCandidate(tn, tn.EXNode, now)
			switch kind {
			case "wrong-tunnel":
				r.TunnelID = "another"
			case "wrong-instance":
				r.InstanceID = "another"
			case "wrong-role":
				r.Role = tunnelnode.RoleIR
			case "missing-digest":
				r.ConfigSHA256 = ""
			case "stale":
				r.ObservedAt = now.Add(-candidateMaxAge - time.Second)
			case "future":
				r.ObservedAt = now.Add(2 * time.Minute)
			case "wrong-route":
				r.RouteID = "another"
			case "wrong-metrics":
				r.MetricsListen = "203.0.113.9:9999"
			case "no-resources":
				r.Resources = false
			case "wrong-generation":
				r.Generation = 2
			}
			b, _ := json.Marshal(r)
			detail := string(b)
			if kind == "plain-success" {
				detail = "candidate ready"
			}
			if kind == "trailing-json" {
				detail += " {}"
			}
			if kind == "unknown-field" {
				detail = strings.TrimSuffix(detail, "}") + `,"ready":true}`
			}
			j := pullAll(t, s, tn.EXNode)[0]
			s.AckJobOutput(tn.EXNode, "tok-"+tn.EXNode, j.ID, "succeeded", detail, "")
			s.AdvanceTunnels(now)
			got, _ := s.GetTunnel(tn.ID)
			if got.Phase != TunnelRollingBack {
				t.Fatalf("unverified success advanced: %+v", got)
			}
			for _, id := range got.Jobs {
				if s.st.Jobs[id].Type == JobTunnelCommitCandidate {
					t.Fatal("activation queued on unverified artifacts")
				}
			}
		})
	}
}

func TestCandidateCommitUsesBothReportsAndRefusesLaterStaleness(t *testing.T) {
	s := tunnelStore(t)
	now := time.Now().UTC()
	tn := newTunnel(t, s, now)
	for _, step := range []struct{ node, out string }{{tn.EXNode, testPairCode}, {tn.IRNode, testReplyCode}} {
		j := pullAll(t, s, step.node)[0]
		s.AckJobOutput(step.node, "tok-"+step.node, j.ID, "succeeded", "ok", step.out)
		s.AdvanceTunnels(now)
	}
	completeCandidateStages(t, s, tn.ID, now)
	j := pullAll(t, s, tn.EXNode)[0]
	if j.Type != JobTunnelCommitCandidate || j.Params["config_sha256"] != strings.Repeat("a", 64) || j.Params["generation"] != "0" || j.Params["reply"] != "" {
		t.Fatalf("commit lease: %+v", j)
	}
	got, _ := s.GetTunnel(tn.ID)
	if len(got.Candidates) != 2 {
		t.Fatal("activation queued without both artifacts")
	}
	s.AckJobOutput(tn.EXNode, "tok-"+tn.EXNode, j.ID, "succeeded", "installed", "")
	s.AdvanceTunnels(now.Add(candidateMaxAge + time.Second))
	got, _ = s.GetTunnel(tn.ID)
	if got.Phase != TunnelRollingBack {
		t.Fatal("stale IR report allowed activation")
	}
}

func TestOlderInflightTunnelKeepsItsExistingCommitProtocol(t *testing.T) {
	s := tunnelStore(t)
	now := time.Now().UTC()
	tn := newTunnel(t, s, now)
	// Reproduce a persisted pre-upgrade receipt, rather than disabling the
	// new protocol for normal test deployments.
	tn.CandidatePreparation = false
	s.st.Tunnels[tn.ID] = tn
	j := s.st.Jobs[tn.JobID]
	delete(j.Params, "stage_config")
	s.st.Jobs[j.ID] = j
	if err := s.saveLocked(); err != nil {
		t.Fatal(err)
	}
	again, err := OpenStore(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct{ node, out string }{{tn.EXNode, testPairCode}, {tn.IRNode, testReplyCode}} {
		j := pullAll(t, again, step.node)[0]
		if j.Params["stage_config"] != "" {
			t.Fatal("old receipt was silently upgraded")
		}
		again.AckJobOutput(step.node, "tok-"+step.node, j.ID, "succeeded", "ok", step.out)
		again.AdvanceTunnels(now)
	}
	j = pullAll(t, again, tn.EXNode)[0]
	if j.Type != JobTunnelCommitEX || j.Params["reply"] != testReplyCode {
		t.Fatalf("older protocol changed: %+v", j)
	}
}
