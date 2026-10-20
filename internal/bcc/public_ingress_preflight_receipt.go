package bcc

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
)

var errPublicIngressPreflightNotFound = errors.New("public ingress preflight not found")

// PublicIngressPreflightReceipt exposes only this read-only job's state and
// validated observations. Generic jobs may carry secrets and are never returned.
type PublicIngressPreflightReceipt struct {
	JobID        string          `json:"job_id"`
	NodeID       string          `json:"node_id"`
	Status       string          `json:"status"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	ReadOnly     bool            `json:"read_only"`
	ReadyToApply bool            `json:"ready_to_apply"`
	Evidence     json.RawMessage `json:"evidence,omitempty"`
}

func (s *Store) PublicIngressPreflightReceipt(id string) (PublicIngressPreflightReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.st.Jobs[id]
	if !ok || j.Type != agentjob.ActionPublicIngressPreflight {
		return PublicIngressPreflightReceipt{}, errPublicIngressPreflightNotFound
	}
	receipt := PublicIngressPreflightReceipt{JobID: j.ID, NodeID: j.NodeID, Status: j.Status, CreatedAt: j.CreatedAt, UpdatedAt: j.UpdatedAt, ReadOnly: true}
	if j.Status == "succeeded" {
		if err := validatePublicIngressPreflightReceipt(j.Output, j.Params); err != nil {
			return PublicIngressPreflightReceipt{}, err
		}
		receipt.Evidence = json.RawMessage(j.Output)
	}
	return receipt, nil
}

// validatePublicIngressPreflightReceipt validates observations, never grants
// ownership or permission to apply. It applies only to successful new receipts.
func validatePublicIngressPreflightReceipt(output string, params map[string]string) error {
	if len(output) == 0 || len(output) > maxJobOutput {
		return errors.New("invalid public ingress preflight receipt size")
	}
	dec := json.NewDecoder(bytes.NewBufferString(output))
	token, err := dec.Token()
	if err != nil || token != json.Delim('{') {
		return errors.New("preflight receipt must be one JSON object")
	}
	fields := map[string]json.RawMessage{}
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return errors.New("invalid preflight receipt field")
		}
		key, ok := token.(string)
		if !ok {
			return errors.New("invalid preflight receipt field")
		}
		if _, duplicate := fields[key]; duplicate {
			return errors.New("duplicate preflight receipt field")
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return errors.New("invalid preflight receipt value")
		}
		fields[key] = raw
	}
	if _, err := dec.Token(); err != nil {
		return errors.New("incomplete preflight receipt")
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return errors.New("preflight receipt has trailing data")
	}
	required := []string{"schema_version", "tunnel_id", "public_port", "local_target", "scope", "socket_inventory_complete", "public_port_state", "public_listener_count", "target_listener_observed", "ownership_verified", "firewall_tools", "firewall_policy_verified", "ready_to_apply", "limitations"}
	if len(fields) != len(required) {
		return errors.New("preflight receipt fields are incomplete or unknown")
	}
	for _, key := range required {
		raw, ok := fields[key]
		if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return errors.New("preflight receipt requires explicit fields")
		}
	}
	var receipt struct {
		SchemaVersion           int      `json:"schema_version"`
		TunnelID                string   `json:"tunnel_id"`
		PublicPort              int      `json:"public_port"`
		LocalTarget             string   `json:"local_target"`
		Scope                   string   `json:"scope"`
		SocketInventoryComplete bool     `json:"socket_inventory_complete"`
		PublicPortState         string   `json:"public_port_state"`
		PublicListenerCount     int      `json:"public_listener_count"`
		TargetListenerObserved  bool     `json:"target_listener_observed"`
		OwnershipVerified       bool     `json:"ownership_verified"`
		FirewallTools           []string `json:"firewall_tools"`
		FirewallPolicyVerified  bool     `json:"firewall_policy_verified"`
		ReadyToApply            bool     `json:"ready_to_apply"`
		Limitations             []string `json:"limitations"`
	}
	if err := json.Unmarshal([]byte(output), &receipt); err != nil {
		return errors.New("malformed preflight receipt")
	}
	port, err := strconv.Atoi(params["public_port"])
	if err != nil || receipt.SchemaVersion != 1 || receipt.Scope != "agent_network_namespace_snapshot" || receipt.TunnelID != params["tunnel_id"] || receipt.PublicPort != port || receipt.LocalTarget != params["local_target"] {
		return errors.New("preflight receipt does not match dispatched job")
	}
	if receipt.ReadyToApply || receipt.OwnershipVerified || receipt.FirewallPolicyVerified {
		return errors.New("preflight receipt cannot authorize application or claim verified ownership or firewall policy")
	}
	switch receipt.PublicPortState {
	case "occupied":
		if receipt.PublicListenerCount < 1 {
			return errors.New("contradictory listener evidence")
		}
	case "not_observed":
		if !receipt.SocketInventoryComplete || receipt.PublicListenerCount != 0 {
			return errors.New("contradictory absent listener evidence")
		}
	case "unknown":
		if receipt.SocketInventoryComplete || receipt.PublicListenerCount != 0 {
			return errors.New("contradictory unknown listener evidence")
		}
	default:
		return errors.New("invalid preflight public port state")
	}
	return nil
}
