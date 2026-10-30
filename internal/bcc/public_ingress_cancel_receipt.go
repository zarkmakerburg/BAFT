package bcc

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

type publicIngressCancelEvidence struct {
	JobID                 string                `json:"job_id"`
	NodeID                string                `json:"node_id"`
	Proof                 ingressmanifest.Proof `json:"proof"`
	ListenerID            string                `json:"listener_id"`
	FirewallID            string                `json:"firewall_id"`
	LiveOwnershipVerified bool                  `json:"live_ownership_verified"`
	ReadyToApply          bool                  `json:"ready_to_apply"`
}

func validatePublicIngressCancelReceipt(output string, job Job, op PublicIngressOperation) error {
	if len(output) == 0 || len(output) > maxJobOutput || op.Status != PublicIngressPending || job.NodeID != op.Spec.IRNode || job.Params["tunnel_id"] != op.Spec.TunnelID {
		return errors.New("invalid ingress cancellation receipt context")
	}
	var receipt publicIngressCancelEvidence
	if err := json.Unmarshal([]byte(output), &receipt); err != nil {
		return errors.New("invalid ingress cancellation receipt")
	}
	canonical, err := json.Marshal(receipt)
	if err != nil || !bytes.Equal([]byte(output), canonical) {
		return errors.New("noncanonical ingress cancellation receipt")
	}
	if receipt.JobID != job.ID || receipt.NodeID != job.NodeID || receipt.ListenerID != op.Proof.ListenerID || receipt.FirewallID != op.Proof.FirewallID || receipt.LiveOwnershipVerified || receipt.ReadyToApply {
		return errors.New("ingress cancellation receipt differs from fenced operation")
	}
	spec := publicIngressManifestSpec(op.Spec, job.Params["instance_id"], op.Proof.Generation)
	prepared, err := ingressmanifest.Preview(spec)
	if err != nil || prepared.ManifestSHA256 != op.Proof.ManifestDigest {
		return errors.New("ingress cancellation prepared proof mismatch")
	}
	expected, err := ingressmanifest.PreviewCancellation(spec)
	if err != nil || receipt.Proof != expected || job.Params["manifest_digest"] != op.Proof.ManifestDigest {
		return errors.New("ingress cancellation tombstone proof mismatch")
	}
	return nil
}
