package bcc

import (
	"encoding/json"
	"errors"
	"strings"
)

// validatePublicIngressInspectSnapshot only accepts a non-authoritative,
// signed-intent echo. It NEVER proves ownership or authorizes APPLIED.
func validatePublicIngressInspectSnapshot(output string, jobID, nodeID string, params map[string]string) error {
	if len(output) == 0 || len(output) > maxJobOutput {
		return errors.New("invalid ingress inspect receipt size")
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		return err
	}
	expected := []string{"preflight", "job_id", "node_id", "generation", "manifest_digest", "listener_id", "firewall_id", "live_ownership_verified", "ready_to_apply"}
	if len(raw) != len(expected) {
		return errors.New("invalid inspect receipt fields")
	}
	for _, key := range expected {
		if _, ok := raw[key]; !ok {
			return errors.New("missing inspect receipt field")
		}
	}
	var receipt struct {
		Preflight             json.RawMessage `json:"preflight"`
		JobID                 string          `json:"job_id"`
		NodeID                string          `json:"node_id"`
		Generation            string          `json:"generation"`
		ManifestDigest        string          `json:"manifest_digest"`
		ListenerID            string          `json:"listener_id"`
		FirewallID            string          `json:"firewall_id"`
		LiveOwnershipVerified bool            `json:"live_ownership_verified"`
		ReadyToApply          bool            `json:"ready_to_apply"`
	}
	if err := json.Unmarshal([]byte(output), &receipt); err != nil {
		return err
	}
	if receipt.JobID != jobID || receipt.NodeID != nodeID || receipt.Generation != params["generation"] || receipt.ManifestDigest != params["manifest_digest"] || receipt.ListenerID != params["listener_id"] || receipt.FirewallID != params["firewall_id"] {
		return errors.New("inspect signed intent mismatch")
	}
	if receipt.LiveOwnershipVerified || receipt.ReadyToApply {
		return errors.New("inspect cannot assert verified ownership or readiness")
	}
	// Reuse preflight's strict field contract: this is still only a socket snapshot.
	if err := validatePublicIngressPreflightReceipt(string(receipt.Preflight), params); err != nil {
		return err
	}
	if strings.TrimSpace(receipt.JobID) == "" || strings.TrimSpace(receipt.NodeID) == "" {
		return errors.New("missing job identity")
	}
	return nil
}
