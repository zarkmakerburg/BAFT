package bcc

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

type publicIngressPrepareEvidence struct {
	JobID                 string                `json:"job_id"`
	NodeID                string                `json:"node_id"`
	Proof                 ingressmanifest.Proof `json:"proof"`
	ListenerID            string                `json:"listener_id"`
	FirewallID            string                `json:"firewall_id"`
	PreparedBytesMatch    bool                  `json:"prepared_bytes_match"`
	LiveOwnershipVerified bool                  `json:"live_ownership_verified"`
	ReadyToApply          bool                  `json:"ready_to_apply"`
}

// Only the exact canonical Agent receipt for the persisted cursor is accepted.
// A prepared file is not a live listener or firewall ownership observation.
func validatePublicIngressPrepareReceipt(output string, job Job, op PublicIngressOperation) error {
	if len(output) == 0 || len(output) > maxJobOutput || job.Params["tunnel_id"] != op.Spec.TunnelID || job.NodeID != op.Spec.IRNode || op.Status != PublicIngressPending {
		return errors.New("invalid ingress prepare receipt context")
	}
	var receipt publicIngressPrepareEvidence
	if err := json.Unmarshal([]byte(output), &receipt); err != nil {
		return errors.New("invalid ingress prepare receipt")
	}
	canonical, err := json.Marshal(receipt)
	if err != nil || !bytes.Equal([]byte(output), canonical) {
		return errors.New("noncanonical ingress prepare receipt")
	}
	if receipt.JobID != job.ID || receipt.NodeID != job.NodeID || receipt.ListenerID != op.Proof.ListenerID || receipt.FirewallID != op.Proof.FirewallID || !receipt.PreparedBytesMatch || receipt.LiveOwnershipVerified || receipt.ReadyToApply {
		return errors.New("ingress prepare receipt does not match fenced operation")
	}
	expected, err := ingressmanifest.Preview(publicIngressManifestSpec(op.Spec, job.Params["instance_id"], op.Proof.Generation))
	if err != nil || receipt.Proof != expected || expected.ManifestSHA256 != op.Proof.ManifestDigest {
		return errors.New("ingress prepared manifest proof mismatch")
	}
	return nil
}
