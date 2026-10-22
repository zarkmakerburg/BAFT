package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

type publicIngressCancelReceipt struct {
	JobID                 string                `json:"job_id"`
	NodeID                string                `json:"node_id"`
	Proof                 ingressmanifest.Proof `json:"proof"`
	ListenerID            string                `json:"listener_id"`
	FirewallID            string                `json:"firewall_id"`
	LiveOwnershipVerified bool                  `json:"live_ownership_verified"`
	ReadyToApply          bool                  `json:"ready_to_apply"`
}

// publicIngressCancel permanently fences an initial prepared generation,
// including one whose prepare ACK was lost. It never touches live resources.
func (a *Agent) publicIngressCancel(ctx context.Context, j agentjob.Job) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if j.Action != agentjob.ActionPublicIngressCancel {
		return "", errors.New("not an ingress cancellation job")
	}
	if err := j.Validate(); err != nil {
		return "", err
	}
	if a.cfg.IngressManifestDir == "" {
		return "", errors.New("ingress manifest directory is not configured")
	}
	generation, err := strconv.ParseUint(j.Params["generation"], 10, 64)
	if err != nil || generation != 1 {
		return "", errors.New("ingress cancellation requires initial generation 1")
	}
	port, _ := strconv.Atoi(j.Params["public_port"])
	spec := ingressmanifest.Spec{TunnelID: j.Params["tunnel_id"], InstanceID: j.Params["instance_id"], Generation: generation, Config: ingressmanifest.Config{
		Listen: net.JoinHostPort("0.0.0.0", strconv.Itoa(port)), Target: j.Params["local_target"], MaxClients: 128, DialTimeoutMS: 3000,
	}}
	expected, err := ingressmanifest.Preview(spec)
	if err != nil || expected.ManifestSHA256 != j.Params["manifest_digest"] {
		return "", errors.New("signed ingress cancellation proof differs from canonical prepared bytes")
	}
	tombstone, err := ingressmanifest.PreviewCancellation(spec)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	store, err := ingressmanifest.Open(a.cfg.IngressManifestDir)
	if err != nil {
		return "", fmt.Errorf("open private ingress manifest store: %w", err)
	}
	defer store.Close()
	proof, err := store.Cancel(expected)
	if errors.Is(err, ingressmanifest.ErrNotPrepared) {
		proof, err = store.FenceUnseen(spec)
	}
	if err != nil {
		return "", err
	}
	if proof != tombstone {
		return "", errors.New("ingress cancellation tombstone differs from signed proof")
	}
	raw, err := json.Marshal(publicIngressCancelReceipt{
		JobID: j.JobID, NodeID: j.NodeID, Proof: proof, ListenerID: j.Params["listener_id"], FirewallID: j.Params["firewall_id"],
	})
	return string(raw), err
}
