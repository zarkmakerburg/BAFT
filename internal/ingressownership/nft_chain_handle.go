package ingressownership

import "encoding/json"

// ReadNFTLabChainHandle returns the handle of the single inet/baft/ingress
// chain in a ruleset snapshot, but only if its layout is the pinned one
// (filter, input hook, priority -100, accept). Anything else is refused.
func ReadNFTLabChainHandle(raw []byte) (uint64, error) {
	var root struct {
		NFTables []map[string]json.RawMessage `json:"nftables"`
	}
	if len(raw) == 0 || len(raw) > 4<<20 || json.Unmarshal(raw, &root) != nil {
		return 0, ErrFirewallUnverified
	}
	var handle uint64
	for _, item := range root.NFTables {
		chainRaw, ok := item["chain"]
		if !ok {
			continue
		}
		var c struct {
			Family string `json:"family"`
			Table  string `json:"table"`
			Name   string `json:"name"`
			Handle uint64 `json:"handle"`
		}
		if json.Unmarshal(chainRaw, &c) != nil {
			return 0, ErrFirewallUnverified
		}
		if c.Family == "inet" && c.Table == "baft" && c.Name == "ingress" {
			if handle != 0 {
				return 0, ErrFirewallUnverified
			}
			handle = c.Handle
		}
	}
	if handle == 0 || verifyNFTLabChain(raw, handle) != nil {
		return 0, ErrFirewallUnverified
	}
	return handle, nil
}
