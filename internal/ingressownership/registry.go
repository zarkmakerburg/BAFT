// Package ingressownership models fail-closed ingress resource ownership.
// This is an in-memory laboratory primitive, NOT an authorization to mutate
// host listeners or firewall rules. Durable reconciliation remains required.
package ingressownership

import (
	"errors"

	"github.com/zarkmakerburg/baft/internal/ingressinventory"
	"net"
	"regexp"
	"strconv"
	"sync"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

var (
	ErrConflict = ingressinventory.ErrClaimConflict
	ErrUnknown  = errors.New("ingress resource ownership is unknown")
	ErrStale    = errors.New("ingress generation is stale")
	ErrInvalid  = errors.New("invalid ingress ownership proof")
)

type Registry struct {
	mu     sync.Mutex
	owners map[string]ingressmanifest.Proof
	// endpoints fences exact listener identity, not just a numeric port;
	// the same proof must never authorize a second IP/family alias.
	endpoints   map[string]string
	generations map[string]uint64
	active      map[string]string
}

func New() *Registry {
	return &Registry{owners: make(map[string]ingressmanifest.Proof), endpoints: make(map[string]string), generations: make(map[string]uint64), active: make(map[string]string)}
}

var ownerTunnelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
var ownerInstancePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,39}$`)

func key(p ingressmanifest.Proof) string { return p.TunnelID + "\x00" + p.InstanceID }
func valid(p ingressmanifest.Proof) bool {
	// The key uses a NUL separator: untrusted IDs containing NUL can alias
	// another tunnel/instance tuple and bypass generation ownership fences.
	if !ownerTunnelPattern.MatchString(p.TunnelID) || (p.InstanceID != "" && !ownerInstancePattern.MatchString(p.InstanceID)) || p.InstanceID == "default" || p.Generation == 0 || len(p.ManifestSHA256) != 64 || len(p.ConfigSHA256) != 64 {
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

// checkClaimLocked checks generation and ownership fences without mutating
// them. Runtime uses this before binding so stale replays fail deterministically
// even when a foreign listener currently owns the port.
func (r *Registry) checkClaimLocked(resource, canonical string, p ingressmanifest.Proof) error {
	if p.Generation < r.generations[key(p)] {
		return ErrStale
	}
	if current, ok := r.active[key(p)]; ok && current != resource {
		return ErrConflict
	}
	if old, ok := r.owners[resource]; ok {
		if old == p && r.endpoints[resource] == canonical {
			return nil
		}
		return ErrConflict
	}
	if p.Generation == r.generations[key(p)] && r.generations[key(p)] != 0 {
		return ErrStale
	}
	return nil
}

func (r *Registry) checkClaim(endpoint string, p ingressmanifest.Proof) error {
	if r == nil || !valid(p) {
		return ErrInvalid
	}
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil || net.ParseIP(host) == nil {
		return ErrInvalid
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return ErrInvalid
	}
	resource := strconv.Itoa(n)
	canonical := net.JoinHostPort(net.ParseIP(host).String(), resource)
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.checkClaimLocked(resource, canonical, p)
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
	if err != nil || n < 1 || n > 65535 {
		return ErrInvalid
	}
	resource := strconv.Itoa(n)
	canonical := net.JoinHostPort(net.ParseIP(host).String(), resource)
	r.mu.Lock()
	defer r.mu.Unlock()
	// Zero-value registries must fail safely rather than panic on Claim.
	if r.owners == nil {
		r.owners = make(map[string]ingressmanifest.Proof)
	}
	if r.endpoints == nil {
		r.endpoints = make(map[string]string)
	}
	if r.generations == nil {
		r.generations = make(map[string]uint64)
	}
	if r.active == nil {
		r.active = make(map[string]string)
	}
	if err := r.checkClaimLocked(resource, canonical, p); err != nil {
		return err
	}
	r.owners[resource] = p
	r.endpoints[resource] = canonical
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
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil || net.ParseIP(host) == nil {
		return ErrInvalid
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return ErrInvalid
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	resource := strconv.Itoa(n)
	canonical := net.JoinHostPort(net.ParseIP(host).String(), resource)
	if r.owners[resource] != p || r.endpoints[resource] != canonical {
		return ErrUnknown
	}
	delete(r.owners, resource)
	delete(r.endpoints, resource)
	delete(r.active, key(p))
	return nil
}
