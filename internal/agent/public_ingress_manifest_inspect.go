package agent

import (
	"errors"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
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
