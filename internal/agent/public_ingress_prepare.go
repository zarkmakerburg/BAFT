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

type publicIngressPrepareReceipt struct {
	JobID                 string                `json:"job_id"`
	NodeID                string                `json:"node_id"`
	Proof                 ingressmanifest.Proof `json:"proof"`
	ListenerID            string                `json:"listener_id"`
	FirewallID            string                `json:"firewall_id"`
	PreparedBytesMatch    bool                  `json:"prepared_bytes_match"`
	LiveOwnershipVerified bool                  `json:"live_ownership_verified"`
	ReadyToApply          bool                  `json:"ready_to_apply"`
}

// publicIngressPrepare writes only a private manifest. Signed parameters and
// the trusted directory are necessary; this never starts a listener or alters
// firewall policy. Generation advancement needs a separate prior-proof flow.
func (a *Agent) publicIngressPrepare(ctx context.Context, j agentjob.Job) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if j.Action != agentjob.ActionPublicIngressPrepare {
		return "", errors.New("not an ingress prepare job")
	}
	if err := j.Validate(); err != nil {
		return "", err
	}
	if a.cfg.IngressManifestDir == "" {
		return "", errors.New("ingress manifest directory is not configured")
	}
	generation, err := strconv.ParseUint(j.Params["generation"], 10, 64)
	if err != nil || generation != 1 {
		return "", errors.New("ingress prepare requires initial generation 1")
	}
	port, _ := strconv.Atoi(j.Params["public_port"])
	spec := ingressmanifest.Spec{TunnelID: j.Params["tunnel_id"], InstanceID: j.Params["instance_id"], Generation: generation, Config: ingressmanifest.Config{
		Listen: net.JoinHostPort("0.0.0.0", strconv.Itoa(port)), Target: j.Params["local_target"], MaxClients: 128, DialTimeoutMS: 3000,
	}}
	expected, err := ingressmanifest.Preview(spec)
	if err != nil {
		return "", err
	}
	if expected.ManifestSHA256 != j.Params["manifest_digest"] {
		return "", errors.New("signed ingress manifest digest differs from canonical prepared bytes")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	store, err := ingressmanifest.Open(a.cfg.IngressManifestDir)
	if err != nil {
		return "", fmt.Errorf("open private ingress manifest store: %w", err)
	}
	defer store.Close()
	proof, err := store.Prepare(spec, nil)
	if err != nil {
		return "", err
	}
	if proof != expected {
		return "", errors.New("prepared ingress manifest differs from signed proof")
	}
	receipt := publicIngressPrepareReceipt{JobID: j.JobID, NodeID: j.NodeID, Proof: proof, ListenerID: j.Params["listener_id"], FirewallID: j.Params["firewall_id"], PreparedBytesMatch: true}
	raw, err := json.Marshal(receipt)
	return string(raw), err
}
