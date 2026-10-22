package ingressownership

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// ParseNFTIngressRules consumes an untrusted nft -j list ruleset snapshot.
// It is strictly read-only. Unknown expressions on the BAFT chain are not
// interpreted as safe ownership; the caller must not authorize mutation.
func ParseNFTIngressRules(raw []byte) ([]FirewallObservation, error) {
	if len(raw) == 0 || len(raw) > 4<<20 {
		return nil, ErrFirewallUnverified
	}
	var root struct {
		NFTables []json.RawMessage `json:"nftables"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&root); err != nil {
		return nil, fmt.Errorf("nft snapshot: %w", ErrFirewallUnverified)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return nil, ErrFirewallUnverified
	}
	if len(root.NFTables) == 0 {
		return nil, ErrFirewallUnverified
	}
	out := make([]FirewallObservation, 0)
	for _, item := range root.NFTables {
		var wrapper map[string]json.RawMessage
		if err := json.Unmarshal(item, &wrapper); err != nil {
			return nil, ErrFirewallUnverified
		}
		ruleRaw, ok := wrapper["rule"]
		if !ok {
			continue
		}
		var rule struct {
			Family  string            `json:"family"`
			Table   string            `json:"table"`
			Chain   string            `json:"chain"`
			Handle  uint64            `json:"handle"`
			Comment string            `json:"comment"`
			Expr    []json.RawMessage `json:"expr"`
		}
		if err := json.Unmarshal(ruleRaw, &rule); err != nil {
			return nil, ErrFirewallUnverified
		}
		if rule.Family != "inet" || rule.Table != "baft" || rule.Chain != "ingress" {
			// An owner tag outside the sole managed chain is ambiguous
			// ownership, not an unrelated rule to silently ignore.
			if strings.HasPrefix(rule.Comment, "baft-ingress-") {
				return nil, ErrFirewallUnverified
			}
			continue
		}
		// Only the exact, minimal nft JSON shape is supported: a TCP dport
		// equality followed by an accept verdict. Anything else is opaque.
		if rule.Handle == 0 || !strings.HasPrefix(rule.Comment, "baft-ingress-") || len(rule.Expr) != 2 {
			return nil, ErrFirewallUnverified
		}
		var portMatch struct {
			Match struct {
				Op   string `json:"op"`
				Left struct {
					Payload struct {
						Protocol string `json:"protocol"`
						Field    string `json:"field"`
					} `json:"payload"`
				} `json:"left"`
				Right uint16 `json:"right"`
			} `json:"match"`
		}
		if err := decodeExactNFTMatch(rule.Expr[0], &portMatch); err != nil || portMatch.Match.Op != "==" || portMatch.Match.Left.Payload.Protocol != "tcp" || portMatch.Match.Left.Payload.Field != "dport" || portMatch.Match.Right == 0 {
			return nil, ErrFirewallUnverified
		}
		var verdict map[string]json.RawMessage
		if err := json.Unmarshal(rule.Expr[1], &verdict); err != nil || len(verdict) != 1 {
			return nil, ErrFirewallUnverified
		}
		if _, ok := verdict["accept"]; !ok {
			return nil, ErrFirewallUnverified
		}
		if string(verdict["accept"]) != "null" {
			return nil, ErrFirewallUnverified
		}
		out = append(out, FirewallObservation{Port: portMatch.Match.Right, Tag: rule.Comment, Handle: rule.Handle, Family: rule.Family, Table: rule.Table, Chain: rule.Chain})
	}
	return out, nil
}

// decodeExactNFTMatch rejects unknown or injected expressions rather than
// silently ignoring extra fields in a firewall ownership proof.
func decodeExactNFTMatch(raw []byte, target any) error {
	var outer map[string]json.RawMessage
	if err := json.Unmarshal(raw, &outer); err != nil {
		return err
	}
	if len(outer) != 1 {
		return ErrFirewallUnverified
	}
	match, ok := outer["match"]
	if !ok {
		return ErrFirewallUnverified
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(match, &fields); err != nil || len(fields) != 3 {
		return ErrFirewallUnverified
	}
	for _, key := range []string{"op", "left", "right"} {
		if _, ok := fields[key]; !ok {
			return ErrFirewallUnverified
		}
	}
	var left map[string]json.RawMessage
	if err := json.Unmarshal(fields["left"], &left); err != nil || len(left) != 1 {
		return ErrFirewallUnverified
	}
	payload, ok := left["payload"]
	if !ok {
		return ErrFirewallUnverified
	}
	var payloadFields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &payloadFields); err != nil || len(payloadFields) != 2 {
		return ErrFirewallUnverified
	}
	if _, ok := payloadFields["protocol"]; !ok {
		return ErrFirewallUnverified
	}
	if _, ok := payloadFields["field"]; !ok {
		return ErrFirewallUnverified
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(target)
}
