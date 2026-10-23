package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// IngressLABExecutor is injected only by an isolated LAB harness. The normal
// Agent has no executor and therefore cannot mutate public ingress resources.
type IngressLABExecutor interface {
	Apply(context.Context, ingressmanifest.Spec, ingressmanifest.Proof) error
	Retire(context.Context, ingressmanifest.Spec, ingressmanifest.Proof) error
}

type publicIngressLABReceipt struct {
	JobID                 string                `json:"job_id"`
	NodeID                string                `json:"node_id"`
	Action                string                `json:"action"`
	Proof                 ingressmanifest.Proof `json:"proof"`
	LiveOwnershipVerified bool                  `json:"live_ownership_verified"`
}

func (a *Agent) publicIngressLAB(ctx context.Context, j agentjob.Job) (string, error) {
	if ctx == nil {
		return "", errors.New("ingress job requires a non-nil context")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if j.Action != agentjob.ActionPublicIngressApply && j.Action != agentjob.ActionPublicIngressRetire {
		return "", errors.New("not a live ingress job")
	}
	if err := j.Validate(); err != nil {
		return "", err
	}
	if a == nil || a.cfg.IngressLABExecutor == nil || a.cfg.IngressManifestDir == "" || a.cfg.NodeID == "" || a.cfg.NodeID != j.NodeID {
		return "", errors.New("isolated ingress LAB execution is not configured for this node")
	}
	generation, err := strconv.ParseUint(j.Params["generation"], 10, 64)
	if err != nil || generation != 1 {
		return "", errors.New("unsupported ingress generation")
	}
	port, err := strconv.Atoi(j.Params["public_port"])
	if err != nil {
		return "", err
	}
	spec := ingressmanifest.Spec{TunnelID: j.Params["tunnel_id"], InstanceID: j.Params["instance_id"], Generation: generation, Config: ingressmanifest.Config{
		Listen: net.JoinHostPort("0.0.0.0", strconv.Itoa(port)), Target: j.Params["local_target"], MaxClients: 128, DialTimeoutMS: 3000,
	}}
	proof, err := ingressmanifest.Preview(spec)
	if err != nil || proof.ManifestSHA256 != j.Params["manifest_digest"] {
		return "", errors.New("signed ingress proof differs from canonical manifest")
	}
	store, err := ingressmanifest.Open(a.cfg.IngressManifestDir)
	if err != nil {
		return "", err
	}
	defer store.Close()
	inspection, err := store.Inspect(proof)
	if err != nil || !inspection.ManifestBytesMatch || !inspection.PreparedBytesMatch || inspection.Manifest.Phase != ingressmanifest.PhasePrepared || inspection.Manifest.Spec != spec {
		return "", errors.New("exact prepared ingress manifest is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if j.Action == agentjob.ActionPublicIngressApply {
		err = a.cfg.IngressLABExecutor.Apply(ctx, spec, proof)
	} else {
		err = a.cfg.IngressLABExecutor.Retire(ctx, spec, proof)
	}
	if err != nil {
		return "", err
	}
	if j.Action == agentjob.ActionPublicIngressApply {
		if ctxErr := ctx.Err(); ctxErr != nil {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			retireErr := a.cfg.IngressLABExecutor.Retire(cleanupCtx, spec, proof)
			return "", errors.Join(ctxErr, retireErr)
		}
		// Cancellation may have fenced the manifest while the LAB executor was
		// binding the listener. A successful executor result is insufficient
		// until the prepared bytes still match after mutation.
		latest, inspectErr := store.Inspect(proof)
		if inspectErr != nil || !latest.PreparedBytesMatch || latest.Manifest.Phase != ingressmanifest.PhasePrepared {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			retireErr := a.cfg.IngressLABExecutor.Retire(cleanupCtx, spec, proof)
			return "", errors.Join(errors.New("prepared ingress proof changed during apply"), inspectErr, retireErr)
		}
	}
	receipt := publicIngressLABReceipt{JobID: j.JobID, NodeID: j.NodeID, Action: j.Action, Proof: proof, LiveOwnershipVerified: j.Action == agentjob.ActionPublicIngressApply}
	raw, err := json.Marshal(receipt)
	return string(raw), err
}
