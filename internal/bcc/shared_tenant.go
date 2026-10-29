package bcc

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/tunnelnode"
)

var (
	tenantMutationIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	tenantPeerIDRe     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	tenantRouteIDRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	tenantNoiseKeyRe   = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
)

type SharedTenantMutationRequest struct {
	TunnelID     string `json:"tunnel_id"`
	Operation    string `json:"operation"`
	NodeID       string `json:"node_id"` // exact config node identity, not the BCC enrollment ID
	Listen       string `json:"listen"`
	PeerID       string `json:"peer_id"`
	PeerPublicKey string `json:"peer_public_key"`
	RouteID      string `json:"route_id"`
	Target       string `json:"target,omitempty"`
}

// CreateSharedTenantMutationJob queues one digest-fenced tenant operation for
// the EX of an active tunnel. Digests are sourced from BCC's accepted tunnel
// evidence, never supplied by the caller.
func (s *Store) CreateSharedTenantMutationJob(in SharedTenantMutationRequest, now time.Time) (Job, error) {
	if !tenantMutationIDRe.MatchString(in.TunnelID) || (in.Operation != "add" && in.Operation != "remove") ||
		strings.TrimSpace(in.NodeID) == "" || strings.TrimSpace(in.Listen) == "" ||
		!tenantPeerIDRe.MatchString(in.PeerID) || !tenantRouteIDRe.MatchString(in.RouteID) ||
		!tenantNoiseKeyRe.MatchString(in.PeerPublicKey) {
		return Job{}, errors.New("invalid shared tenant mutation request")
	}
	host, port, err := net.SplitHostPort(in.Listen)
	if err != nil || net.ParseIP(host) == nil || port == "" || port == "0" {
		return Job{}, errors.New("listener must be a literal IP:port")
	}
	if in.Operation == "add" {
		targetHost, targetPort, err := net.SplitHostPort(in.Target)
		if err != nil || net.ParseIP(targetHost) == nil || targetPort == "" || targetPort == "0" {
			return Job{}, errors.New("tenant add target must be a fixed IP:port")
		}
	} else if in.Target != "" {
		return Job{}, errors.New("tenant remove must not include a target")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.st.Tunnels[in.TunnelID]
	if !ok || t.Phase != TunnelActive {
		return Job{}, errors.New("shared tenant changes require an active managed tunnel")
	}
	digests, ok := t.Digests[t.EXNode]
	if !ok || !validSHA256Digest(digests.Config) || !validSHA256Digest(digests.Marker) || !validSHA256Digest(digests.Unit) {
		return Job{}, errors.New("BCC has no complete accepted EX ownership digests")
	}
	node, ok := s.st.Nodes[t.EXNode]
	if !ok || node.Revoked {
		return Job{}, errors.New("EX node is missing or revoked")
	}
	for _, existing := range s.st.Jobs {
		if existing.Params["tunnel_id"] == in.TunnelID &&
			(existing.Type == agentjob.ActionTunnelTenantAdd || existing.Type == agentjob.ActionTunnelTenantRemove) &&
			(existing.Status == "queued" || existing.Status == "dispatched") {
			return Job{}, errors.New("another shared tenant mutation is still pending for this tunnel")
		}
	}

	var before state
	before, err = cloneState(s.st)
	if err != nil {
		return Job{}, err
	}
	mutationID := newTunnelID()
	action := agentjob.ActionTunnelTenantAdd
	params := map[string]string{
		"tunnel_id": in.TunnelID, "mutation_id": mutationID, "node_id": in.NodeID,
		"listen": in.Listen, "config_sha256": digests.Config, "marker_sha256": digests.Marker,
		"peer_id": in.PeerID, "peer_public_key": in.PeerPublicKey, "route_id": in.RouteID,
	}
	if in.Operation == "add" {
		params["target"] = in.Target
	} else {
		action = agentjob.ActionTunnelTenantRemove
	}
	if t.InstanceID != "" {
		params["instance_id"] = t.InstanceID
		params["metrics_listen"] = t.EXMetricsListen
	}
	job := s.newJobLocked(Job{Type: action, NodeID: t.EXNode, Params: params})
	if err := s.saveLocked(); err != nil {
		s.st = before
		return Job{}, err
	}
	return job, nil
}

func validSHA256Digest(v string) bool {
	if len(v) != 64 {
		return false
	}
	b, err := hex.DecodeString(v)
	return err == nil && len(b) == 32
}

// ReconcileSharedTenantMutationAcks adopts the authenticated agent's receipt
// only when it matches the exact pre-change digests BCC issued and the node's
// expected next generation. It is idempotent, so a failed BCC write can retry.
func (s *Store) ReconcileSharedTenantMutationAcks(now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, job := range s.st.Jobs {
		if job.Type != agentjob.ActionTunnelTenantAdd && job.Type != agentjob.ActionTunnelTenantRemove {
			continue
		}
		if job.Status != "succeeded" {
			continue
		}
		var ev tunnelnode.TenantMutationEvidence
		if err := json.Unmarshal([]byte(job.Output), &ev); err != nil {
			return fmt.Errorf("tenant mutation job %s has invalid receipt: %w", job.ID, err)
		}
		p := job.Params
		tunnelID := p["tunnel_id"]
		t, ok := s.st.Tunnels[tunnelID]
		if !ok || t.Phase != TunnelActive {
			return fmt.Errorf("tenant mutation job %s refers to a non-active tunnel", job.ID)
		}
		digests := t.Digests[t.EXNode]
		if digests.Config == ev.AfterConfigSHA && digests.Marker == ev.AfterMarkerSHA &&
			t.ObservedGen[t.EXNode] == ev.Generation {
			continue // already reconciled
		}
		wantOperation := "add"
		if job.Type == agentjob.ActionTunnelTenantRemove {
			wantOperation = "remove"
		}
		if ev.ID != p["mutation_id"] || ev.Operation != wantOperation || ev.NodeID != p["node_id"] ||
			ev.BeforeConfigSHA != p["config_sha256"] || ev.BeforeMarkerSHA != p["marker_sha256"] ||
			!validSHA256Digest(ev.AfterConfigSHA) || !validSHA256Digest(ev.AfterMarkerSHA) {
			return fmt.Errorf("tenant mutation job %s receipt does not match the signed request", job.ID)
		}
		if digests.Config != ev.BeforeConfigSHA || digests.Marker != ev.BeforeMarkerSHA {
			return fmt.Errorf("tenant mutation job %s pre-change digests no longer match BCC state", job.ID)
		}
		if t.ObservedGen[t.EXNode]+1 != ev.Generation {
			return fmt.Errorf("tenant mutation job %s generation is not the expected next generation", job.ID)
		}
		before, err := cloneState(s.st)
		if err != nil {
			return err
		}
		t.Digests[t.EXNode] = NodeDigests{Config: ev.AfterConfigSHA, Unit: digests.Unit, Marker: ev.AfterMarkerSHA}
		if t.ObservedGen == nil {
			t.ObservedGen = map[string]int{}
		}
		t.ObservedGen[t.EXNode] = ev.Generation
		t.Evidence = append(t.Evidence, TunnelEvidence{
			Step: "shared_tenant_" + wantOperation, Node: t.EXNode, At: now.UTC(), OK: true,
			Detail: fmt.Sprintf("authenticated tenant mutation committed at generation %d", ev.Generation),
		})
		t.UpdatedAt = now.UTC()
		s.st.Tunnels[tunnelID] = t
		n := s.st.Nodes[t.EXNode]
		if t.InstanceID == "" {
			n.AppliedGeneration = ev.Generation
		} else {
			if n.AppliedGenerations == nil {
				n.AppliedGenerations = map[string]int{}
			}
			n.AppliedGenerations[t.InstanceID] = ev.Generation
		}
		s.st.Nodes[t.EXNode] = n
		if err := s.saveLocked(); err != nil {
			s.st = before
			return err
		}
	}
	return nil
}
