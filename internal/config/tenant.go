package config

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// AddListenerTenant adds one authenticated peer and its inbound route to an
// existing BAFT listener config. Callers must verify the live config digest,
// ownership marker, service identity and expected listener address before
// invoking this pure transformation, then write/restart transactionally.
// Repeating an identical request is idempotent; conflicting reuse is refused.
func AddListenerTenant(current Config, expectedNodeID, expectedListen, peerID, peerPublicKey, routeID, target string) (Config, error) {
	if err := validateTenantMutation(current, expectedNodeID, expectedListen, peerID, routeID, target); err != nil {
		return Config{}, err
	}
	if current.Node.Role != "listener" || current.Server == nil || current.Noise == nil {
		return Config{}, errors.New("tenant join requires a Noise-authenticated listener")
	}
	if strings.TrimSpace(peerPublicKey) == "" {
		return Config{}, errors.New("tenant public key is required")
	}

	next := cloneTenantConfig(current)
	if next.Noise.AllowedPeerPublicKeys == nil {
		// Upgrade a legacy single-peer listener to the explicit allowlist form
		// without changing its identity, key, TLS settings, or listener.
		if next.Noise.PeerPublicKey == "" || len(next.Server.AllowedPeerIdentities) != 1 {
			return Config{}, errors.New("listener's existing peer ownership cannot be verified")
		}
		next.Noise.AllowedPeerPublicKeys = map[string]string{
			next.Server.AllowedPeerIdentities[0]: next.Noise.PeerPublicKey,
		}
		next.Noise.PeerPublicKey = ""
	}
	if old, ok := next.Noise.AllowedPeerPublicKeys[peerID]; ok {
		if old == peerPublicKey && tenantRouteMatches(next.Routes, routeID, peerID, target) {
			return next, nil
		}
		return Config{}, fmt.Errorf("tenant %q already exists with different key or route", peerID)
	}
	for id, key := range next.Noise.AllowedPeerPublicKeys {
		if key == peerPublicKey {
			return Config{}, fmt.Errorf("public key is already assigned to tenant %q", id)
		}
	}
	for _, id := range next.Server.AllowedPeerIdentities {
		if id == peerID {
			return Config{}, errors.New("listener identity exists without a matching Noise key")
		}
	}

	next.Noise.AllowedPeerPublicKeys[peerID] = peerPublicKey
	next.Server.AllowedPeerIdentities = append(next.Server.AllowedPeerIdentities, peerID)
	sort.Strings(next.Server.AllowedPeerIdentities)

	// Routes sharing an ID are permitted only for inbound routes with disjoint
	// authenticated peer sets. Reuse the route ID but never another tenant's
	// target or AllowedPeers slice.
	for _, route := range next.Routes {
		if route.ID == routeID && route.Direction != "inbound" {
			return Config{}, fmt.Errorf("route %q is owned by a non-inbound route", routeID)
		}
		if route.ID == routeID && containsTenant(route.AllowedPeers, peerID) {
			return Config{}, fmt.Errorf("route %q already contains tenant %q", routeID, peerID)
		}
	}
	next.Routes = append(next.Routes, Route{
		ID: routeID, Direction: "inbound", Target: target, AllowedPeers: []string{peerID},
	})
	if err := Validate(next); err != nil {
		return Config{}, fmt.Errorf("joined listener config is invalid: %w", err)
	}
	return next, nil
}

// RemoveListenerTenant removes exactly one peer and its route membership.
// It refuses stale keys, ambiguous route ownership, and removal of the final
// authenticated peer. The caller must verify the current digest and commit the
// resulting config using its durable transaction/rollback mechanism.
func RemoveListenerTenant(current Config, expectedNodeID, expectedListen, peerID, expectedPublicKey, routeID string) (Config, error) {
	if current.Node.ID != expectedNodeID || current.Server == nil || current.Noise == nil ||
		current.Node.Role != "listener" || current.Server.Listen != expectedListen {
		return Config{}, errors.New("listener identity or address changed")
	}
	if strings.TrimSpace(peerID) == "" || strings.TrimSpace(routeID) == "" || strings.TrimSpace(expectedPublicKey) == "" {
		return Config{}, errors.New("tenant identity, key and route are required")
	}
	next := cloneTenantConfig(current)
	if next.Noise.AllowedPeerPublicKeys == nil {
		if len(next.Server.AllowedPeerIdentities) != 1 || next.Server.AllowedPeerIdentities[0] != peerID ||
			next.Noise.PeerPublicKey != expectedPublicKey {
			return Config{}, errors.New("legacy listener does not match the requested tenant")
		}
		return Config{}, errors.New("refusing to remove the final authenticated listener tenant")
	}
	actualKey, ok := next.Noise.AllowedPeerPublicKeys[peerID]
	if !ok || actualKey != expectedPublicKey {
		return Config{}, errors.New("tenant key does not match current listener ownership")
	}
	if len(next.Server.AllowedPeerIdentities) <= 1 {
		return Config{}, errors.New("refusing to remove the final authenticated listener tenant")
	}
	matchedRoute := false
	routes := make([]Route, 0, len(next.Routes))
	for _, route := range next.Routes {
		if route.ID != routeID || !containsTenant(route.AllowedPeers, peerID) {
			routes = append(routes, route)
			continue
		}
		if route.Direction != "inbound" || route.Target == "" {
			return Config{}, errors.New("tenant route ownership is not an inbound route")
		}
		matchedRoute = true
		route.AllowedPeers = removeTenant(route.AllowedPeers, peerID)
		if len(route.AllowedPeers) > 0 {
			routes = append(routes, route)
		}
	}
	if !matchedRoute {
		return Config{}, errors.New("tenant route not found; refusing partial removal")
	}
	delete(next.Noise.AllowedPeerPublicKeys, peerID)
	next.Server.AllowedPeerIdentities = removeTenant(next.Server.AllowedPeerIdentities, peerID)
	next.Routes = routes
	if err := Validate(next); err != nil {
		return Config{}, fmt.Errorf("removed listener config is invalid: %w", err)
	}
	return next, nil
}

func validateTenantMutation(c Config, nodeID, listen, peerID, routeID, target string) error {
	if nodeID == "" || c.Node.ID != nodeID || c.Node.Role != "listener" || c.Server == nil || c.Noise == nil {
		return errors.New("expected listener identity does not match current config")
	}
	if listen == "" || c.Server.Listen != listen {
		return errors.New("expected listener address does not match current config")
	}
	if peerID == "" || routeID == "" || strings.TrimSpace(target) == "" {
		return errors.New("tenant identity, route and target are required")
	}
	if err := validateFixedTarget(target); err != nil {
		return fmt.Errorf("tenant target: %w", err)
	}
	return nil
}

func tenantRouteMatches(routes []Route, routeID, peerID, target string) bool {
	for _, route := range routes {
		if route.ID == routeID && route.Direction == "inbound" && route.Target == target && containsTenant(route.AllowedPeers, peerID) {
			return true
		}
	}
	return false
}

func containsTenant(ids []string, id string) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}

func removeTenant(ids []string, id string) []string {
	out := make([]string, 0, len(ids))
	for _, candidate := range ids {
		if candidate != id {
			out = append(out, candidate)
		}
	}
	return out
}

func cloneTenantConfig(c Config) Config {
	out := c
	if c.Server != nil {
		server := *c.Server
		server.AllowedPeerIdentities = append([]string(nil), c.Server.AllowedPeerIdentities...)
		out.Server = &server
	}
	if c.Noise != nil {
		noise := *c.Noise
		if c.Noise.AllowedPeerPublicKeys != nil {
			noise.AllowedPeerPublicKeys = make(map[string]string, len(c.Noise.AllowedPeerPublicKeys))
			for id, key := range c.Noise.AllowedPeerPublicKeys {
				noise.AllowedPeerPublicKeys[id] = key
			}
		}
		out.Noise = &noise
	}
	out.Routes = make([]Route, len(c.Routes))
	for i, route := range c.Routes {
		out.Routes[i] = route
		out.Routes[i].AllowedPeers = append([]string(nil), route.AllowedPeers...)
	}
	return out
}
