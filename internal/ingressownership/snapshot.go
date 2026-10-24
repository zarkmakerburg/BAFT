package ingressownership

import (
	"net"
	"sort"
	"strconv"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// FenceSnapshot is a data-only checkpoint for LAB recovery tests. It is not a
// durable journal, socket proof, firewall proof, or permission to deploy.
type FenceSnapshot struct {
	Generations []GenerationFence `json:"generations"`
	Active      []ActiveFence     `json:"active"`
}
type GenerationFence struct {
	TunnelID   string `json:"tunnel_id"`
	InstanceID string `json:"instance_id"`
	Generation uint64 `json:"generation"`
}
type ActiveFence struct {
	Port  uint16                `json:"port"`
	Proof ingressmanifest.Proof `json:"proof"`
}

// Snapshot copies state while holding the registry lock; no caller receives
// a reference to mutable maps.
func (r *Registry) Snapshot() (FenceSnapshot, error) {
	if r == nil {
		return FenceSnapshot{}, ErrInvalid
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out FenceSnapshot
	for k, g := range r.generations {
		// A NUL separator is unambiguous because Claim validates owner IDs.
		index := -1
		for i := 0; i < len(k); i++ {
			if k[i] == 0 {
				index = i
				break
			}
		}
		if index < 1 || g == 0 {
			return FenceSnapshot{}, ErrInvalid
		}
		out.Generations = append(out.Generations, GenerationFence{k[:index], k[index+1:], g})
	}
	if len(r.active) != len(r.owners) || len(r.endpoints) != len(r.owners) {
		return FenceSnapshot{}, ErrInvalid
	}
	for port, p := range r.owners {
		if !valid(p) || r.generations[key(p)] != p.Generation || r.active[key(p)] != port {
			return FenceSnapshot{}, ErrInvalid
		}
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return FenceSnapshot{}, ErrInvalid
		}
		canonical, err := canonicalEndpoint(r.endpoints[port])
		if err != nil || canonical != r.endpoints[port] {
			return FenceSnapshot{}, ErrInvalid
		}
		_, endpointPort, err := net.SplitHostPort(canonical)
		if err != nil || endpointPort != port {
			return FenceSnapshot{}, ErrInvalid
		}
		out.Active = append(out.Active, ActiveFence{uint16(n), p})
	}
	sort.Slice(out.Generations, func(i, j int) bool {
		a, b := out.Generations[i], out.Generations[j]
		if a.TunnelID != b.TunnelID {
			return a.TunnelID < b.TunnelID
		}
		return a.InstanceID < b.InstanceID
	})
	sort.Slice(out.Active, func(i, j int) bool { return out.Active[i].Port < out.Active[j].Port })
	return out, nil
}

// RecoverFences validates a checkpoint and retains only generation high-water
// marks. Active resources are intentionally NOT adopted after a restart.
// Callers must reconcile live host resources using independent evidence.
func RecoverFences(snapshot FenceSnapshot) (*Registry, error) {
	r := New()
	for _, g := range snapshot.Generations {
		p := ingressmanifest.Proof{TunnelID: g.TunnelID, InstanceID: g.InstanceID, Generation: g.Generation, ConfigSHA256: zeroDigest, ManifestSHA256: zeroDigest}
		if !valid(p) || r.generations[key(p)] != 0 {
			return nil, ErrInvalid
		}
		r.generations[key(p)] = g.Generation
	}
	seenPorts := make(map[uint16]bool)
	seenOwners := make(map[string]bool)
	for _, a := range snapshot.Active {
		if a.Port == 0 || !valid(a.Proof) || seenPorts[a.Port] || seenOwners[key(a.Proof)] || r.generations[key(a.Proof)] != a.Proof.Generation {
			return nil, ErrInvalid
		}
		seenPorts[a.Port] = true
		seenOwners[key(a.Proof)] = true
	}
	return r, nil
}

const zeroDigest = "0000000000000000000000000000000000000000000000000000000000000000"
