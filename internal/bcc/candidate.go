package bcc

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/zarkmakerburg/baft/internal/tunnelnode"
)

const candidateMaxAge = 5 * time.Minute

func candidateJobMatches(t Tunnel, j Job) bool {
	var action, node string
	switch t.Phase {
	case TunnelStagingEX:
		action, node = JobTunnelStageEX, t.EXNode
	case TunnelValidatingIR:
		action, node = JobTunnelValidateCandidate, t.IRNode
	case TunnelCommittingEX:
		action, node = JobTunnelCommitCandidate, t.EXNode
	case TunnelCommittingIR:
		action, node = JobTunnelCommitCandidate, t.IRNode
	default:
		return true
	}
	if j.Type != action || j.NodeID != node || j.Params["tunnel_id"] != t.ID || j.Params["instance_id"] != t.InstanceID {
		return false
	}
	if action == JobTunnelCommitCandidate {
		r, ok := t.Candidates[node]
		return ok && j.Params["config_sha256"] == r.ConfigSHA256 && j.Params["generation"] == strconv.Itoa(r.Generation)
	}
	return true
}

func verifyCandidateReport(t Tunnel, node, role, detail string, now time.Time) (tunnelnode.CandidateReport, []string) {
	var r tunnelnode.CandidateReport
	d := json.NewDecoder(strings.NewReader(detail))
	d.DisallowUnknownFields()
	if len(detail) > 8192 || d.Decode(&r) != nil || d.Decode(&struct{}{}) != io.EOF {
		return r, []string{"candidate artifact report is unreadable"}
	}
	var problems []string
	if r.Version != 1 || r.TunnelID != t.ID || r.InstanceID != t.InstanceID || r.Role != role || (role == tunnelnode.RoleEX && node != t.EXNode) || (role == tunnelnode.RoleIR && node != t.IRNode) {
		problems = append(problems, "candidate report does not match the selected tunnel, instance or node role")
	}
	if r.ObservedAt.IsZero() || now.Sub(r.ObservedAt) > candidateMaxAge || r.ObservedAt.After(now.Add(time.Minute)) {
		problems = append(problems, "candidate artifact report is stale or has an invalid clock")
	}
	if b, err := hex.DecodeString(r.ConfigSHA256); err != nil || len(b) != 32 || r.ConfigSHA256 != strings.ToLower(r.ConfigSHA256) {
		problems = append(problems, "candidate config digest is missing or invalid")
	}
	if exp, ok := t.ExpectedGen[node]; !ok || r.Generation < 0 || (!exp.Bootstrap && r.Generation != exp.From) {
		problems = append(problems, "candidate generation differs from the reviewed plan")
	}
	metrics := t.EXMetricsListen
	if role == tunnelnode.RoleIR {
		metrics = t.IRMetricsListen
	}
	// Legacy singleton metrics are configured by the Agent environment;
	// the reviewed request cannot declare/override them. Scoped plans must
	// match their explicit endpoint. Always require a valid loopback bind.
	if !r.Resources || r.RouteID != t.RouteID || r.RecordShaping != t.RecordShaping || validateLoopbackEndpoint(r.MetricsListen, "candidate metrics") != nil || (metrics != "" && r.MetricsListen != metrics) {
		problems = append(problems, "candidate parameters or resource check differ from the reviewed plan")
	}
	if role == tunnelnode.RoleEX {
		if r.Listen != net.JoinHostPort("0.0.0.0", strconv.Itoa(t.Port)) || r.Target != t.Target || r.PeerAddress != "" || r.RouteListen != "" {
			problems = append(problems, "candidate EX listener or target differs from the plan")
		}
	} else if r.PeerAddress != net.JoinHostPort(t.PublicAddress, strconv.Itoa(t.Port)) || r.RouteListen != t.RouteListen || r.Target != "" || r.Listen != "" {
		problems = append(problems, "candidate IR peer or local route differs from the plan")
	}
	return r, problems
}

func (s *Store) candidatesReadyLocked(t *Tunnel, now time.Time) bool {
	for _, side := range []struct{ node, role string }{{t.EXNode, tunnelnode.RoleEX}, {t.IRNode, tunnelnode.RoleIR}} {
		r, ok := t.Candidates[side.node]
		b, _ := json.Marshal(r)
		_, problems := verifyCandidateReport(*t, side.node, side.role, string(b), now)
		if !ok || len(problems) > 0 {
			s.failLocked(t, "both candidate artifact reports must be complete and fresh before activation", now)
			return false
		}
	}
	return true
}

func (s *Store) queueCandidateCommitLocked(t *Tunnel, node string, now time.Time) {
	r := t.Candidates[node]
	s.tunnelJobLocked(t, node, JobTunnelCommitCandidate, map[string]string{"config_sha256": r.ConfigSHA256, "generation": strconv.Itoa(r.Generation)}, now)
}
