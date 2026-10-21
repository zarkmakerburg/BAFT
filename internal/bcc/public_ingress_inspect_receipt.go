package bcc

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// validatePublicIngressInspectSnapshot only accepts a non-authoritative,
// signed-intent echo. It NEVER proves ownership or authorizes APPLIED.
func validatePublicIngressInspectSnapshot(output string, jobID, nodeID string, params map[string]string) error {
	if len(output) == 0 || len(output) > maxJobOutput {
		return errors.New("invalid ingress inspect receipt size")
	}
	dec := json.NewDecoder(bytes.NewBufferString(output))
	start, err := dec.Token()
	if err != nil || start != json.Delim('{') {
		return errors.New("inspect receipt must be a JSON object")
	}
	raw := make(map[string]json.RawMessage)
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return errors.New("invalid inspect receipt key")
		}
		key, ok := token.(string)
		if !ok {
			return errors.New("invalid inspect receipt key")
		}
		if _, exists := raw[key]; exists {
			return errors.New("duplicate inspect receipt key")
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return errors.New("invalid inspect receipt value")
		}
		raw[key] = value
	}
	if end, err := dec.Token(); err != nil || end != json.Delim('}') {
		return errors.New("invalid inspect receipt end")
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return errors.New("trailing inspect receipt data")
	}
	expected := []string{"preflight", "job_id", "node_id", "generation", "manifest_digest", "listener_id", "firewall_id", "live_ownership_verified", "ready_to_apply"}
	if len(raw) != len(expected) {
		return errors.New("invalid inspect receipt fields")
	}
	for _, key := range expected {
		value, ok := raw[key]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
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
