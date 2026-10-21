package agent

import (
	"errors"
	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"net"
	"strconv"
)

// ingressManifestEvidence is trusted-local, read-only evidence about prepared
// bytes. It deliberately cannot claim a running listener or firewall owner.
type ingressManifestEvidence struct {
	Prepared              bool `json:"prepared"`
	BytesVerified         bool `json:"bytes_verified"`
	LiveOwnershipVerified bool `json:"live_ownership_verified"`
	ReadyToApply          bool `json:"ready_to_apply"`
}

func inspectPreparedIngressManifest(store *ingressmanifest.Store, expected ingressmanifest.Proof) (ingressManifestEvidence, error) {
	if store == nil {
		return ingressManifestEvidence{}, errors.New("trusted ingress manifest store unavailable")
	}
	result, err := store.Inspect(expected)
	if err != nil {
		return ingressManifestEvidence{}, err
	}
	if !result.ManifestBytesMatch || !result.PreparedBytesMatch || result.Manifest.Phase != ingressmanifest.PhasePrepared {
		return ingressManifestEvidence{}, errors.New("ingress manifest is not prepared and verified")
	}
	return ingressManifestEvidence{Prepared: true, BytesVerified: true}, nil
}

// inspectSignedIngressManifest requires an independently held expected proof.
// Signed intent supplies correlation, not proof of stored bytes or host ownership.
func inspectSignedIngressManifest(store *ingressmanifest.Store, job agentjob.Job, expected ingressmanifest.Proof) (ingressManifestEvidence, error) {
	if job.Action != agentjob.ActionPublicIngressInspect {
		return ingressManifestEvidence{}, errors.New("wrong ingress inspection action")
	}
	if err := job.Validate(); err != nil {
		return ingressManifestEvidence{}, err
	}
	generation, err := strconv.ParseUint(job.Params["generation"], 10, 64)
	if err != nil || expected.Generation != generation || expected.TunnelID != job.Params["tunnel_id"] || expected.ManifestSHA256 != job.Params["manifest_digest"] || expected.InstanceID == "default" {
		return ingressManifestEvidence{}, errors.New("signed ingress intent differs from independently retained manifest proof")
	}
	inspection, err := store.Inspect(expected)
	if err != nil {
		return ingressManifestEvidence{}, err
	}
	spec := inspection.Manifest.Spec
	_, port, err := net.SplitHostPort(spec.Config.Listen)
	if err != nil || port != job.Params["public_port"] || spec.Config.Target != job.Params["local_target"] || !inspection.ManifestBytesMatch || !inspection.PreparedBytesMatch || inspection.Manifest.Phase != ingressmanifest.PhasePrepared {
		return ingressManifestEvidence{}, errors.New("signed ingress endpoint differs from verified prepared manifest")
	}
	return ingressManifestEvidence{Prepared: true, BytesVerified: true}, nil
}
