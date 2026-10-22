package ingressownership

import (
	"bytes"
	"encoding/json"
	"fmt"
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
			continue
		}
		// A comment alone cannot prove a rule's TCP destination port. Do not
		// fabricate an observation from opaque nft expression structures.
		return nil, ErrFirewallUnverified
	}
	return out, nil
}
