// Package ingressownership models fail-closed ingress resource ownership.
// This is an in-memory laboratory primitive, NOT an authorization to mutate
// host listeners or firewall rules. Durable reconciliation remains required.
package ingressownership

import (
	"errors"
	"net"
	"strconv"
	"sync"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

var (
	ErrConflict = errors.New("ingress resource is owned by another generation")
	ErrUnknown  = errors.New("ingress resource ownership is unknown")
	ErrStale    = errors.New("ingress generation is stale")
	ErrInvalid  = errors.New("invalid ingress ownership proof")
)

type Registry struct {
	mu          sync.Mutex
	owners      map[string]ingressmanifest.Proof
	generations map[string]uint64
	active      map[string]string
}

func New() *Registry {
	return &Registry{owners: make(map[string]ingressmanifest.Proof), generations: make(map[string]uint64), active: make(map[string]string)}
}

func key(p ingressmanifest.Proof) string { return p.TunnelID + "\x00" + p.InstanceID }
func valid(p ingressmanifest.Proof) bool {
	if p.TunnelID == "" || p.Generation == 0 || len(p.ManifestSHA256) != 64 || len(p.ConfigSHA256) != 64 {
		return false
	}
	for _, d := range []string{p.ManifestSHA256, p.ConfigSHA256} {
		for _, c := range d {
			if c < '0' || c > '9' && c < 'a' || c > 'f' {
				return false
			}
		}
	}
	return true
}

// Claim reserves the entire TCP port, intentionally rejecting even IPv4/IPv6
// wildcard aliases. It is conservative until OS socket ownership is proven.
func (r *Registry) Claim(endpoint string, p ingressmanifest.Proof) error {
	if r == nil || !valid(p) {
		return ErrInvalid
	}
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil || net.ParseIP(host) == nil {
		return ErrInvalid
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1024 || n > 65535 {
		return ErrInvalid
	}
	resource := strconv.Itoa(n)
	r.mu.Lock()
	defer r.mu.Unlock()
	if p.Generation < r.generations[key(p)] {
		return ErrStale
	}
	if current, ok := r.active[key(p)]; ok && current != resource {
		return ErrConflict
	}
	if old, ok := r.owners[resource]; ok {
		if old == p {
			return nil
		}
		return ErrConflict
	}
	if p.Generation == r.generations[key(p)] && r.generations[key(p)] != 0 {
		return ErrStale
	}
	r.owners[resource] = p
	r.active[key(p)] = resource
	r.generations[key(p)] = p.Generation
	return nil
}

// Release only relinquishes an exact proof; unknown ownership fails closed.
// A released generation cannot be claimed again, preventing ABA replay.
func (r *Registry) Release(endpoint string, p ingressmanifest.Proof) error {
	if r == nil || !valid(p) {
		return ErrInvalid
	}
	_, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		return ErrInvalid
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1024 || n > 65535 {
		return ErrInvalid
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	resource := strconv.Itoa(n)
	if r.owners[resource] != p {
		return ErrUnknown
	}
	delete(r.owners, resource)
	delete(r.active, key(p))
	return nil
}
