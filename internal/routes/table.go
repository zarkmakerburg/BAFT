package routes

import (
	"errors"
	"fmt"
	"net"
	"sync"
)

type Route struct {
	ID           string
	Target       string
	AllowedPeers map[string]struct{}
}

// Table resolves (authenticated peer identity, route id) to a fixed target.
// Route ids are scoped per tenant: the same id may appear more than once as
// long as the allowed-peer sets are disjoint, so no peer can match two
// entries. The table can be replaced atomically while sessions are live.
type Table struct {
	mu     sync.RWMutex
	routes map[string][]Route
}

func build(defs []Route) (map[string][]Route, error) {
	m := make(map[string][]Route, len(defs))
	for _, r := range defs {
		if r.ID == "" || len(r.ID) > 64 {
			return nil, errors.New("route id must be 1..64 bytes")
		}
		host, port, err := net.SplitHostPort(r.Target)
		if err != nil || port == "" || port == "0" || net.ParseIP(host) == nil {
			return nil, fmt.Errorf("route %s target must be fixed IP:port", r.ID)
		}
		if len(r.AllowedPeers) == 0 {
			return nil, fmt.Errorf("route %s has empty peer allowlist", r.ID)
		}
		for _, other := range m[r.ID] {
			for p := range r.AllowedPeers {
				if _, dup := other.AllowedPeers[p]; dup {
					return nil, fmt.Errorf("duplicate route: %s", r.ID)
				}
			}
		}
		m[r.ID] = append(m[r.ID], r)
	}
	return m, nil
}

func New(defs []Route) (*Table, error) {
	m, err := build(defs)
	if err != nil {
		return nil, err
	}
	return &Table{routes: m}, nil
}

// Replace installs a new route set atomically. On error the old set stays.
func (t *Table) Replace(defs []Route) error {
	m, err := build(defs)
	if err != nil {
		return err
	}
	t.mu.Lock()
	t.routes = m
	t.mu.Unlock()
	return nil
}

func (t *Table) Resolve(peerID, routeID string) (string, error) {
	t.mu.RLock()
	rs, ok := t.routes[routeID]
	t.mu.RUnlock()
	if !ok {
		return "", errors.New("ROUTE_NOT_FOUND")
	}
	for _, r := range rs {
		if _, ok := r.AllowedPeers[peerID]; ok {
			return r.Target, nil
		}
	}
	return "", errors.New("ROUTE_DENIED")
}
