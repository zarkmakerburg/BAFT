//go:build unix

package ingressauthority

import (
	"context"
	"strings"

	"github.com/zarkmakerburg/baft/internal/ingressinventory"
)

// InstalledPolicyIdentityV1 is a durable INERT identity pinned to an existing
// pending resource binding. It is not source coverage, a lease, or a permit.
// Only the fixed installed-policy loader supplies this observation; callers
// cannot pass a policy, pathname, roots or a purported completeness flag.
type InstalledPolicyIdentityV1 struct {
	NodeID           string `json:"node_id"`
	ProfilePath      string `json:"profile_path"`
	ProfileSHA256    string `json:"profile_sha256"`
	ProfileDevice    uint64 `json:"profile_device"`
	ProfileInode     uint64 `json:"profile_inode"`
	NetworkNamespace string `json:"netns"`
	SchemaVersion    int    `json:"schema_version"`
	InventoryVersion int    `json:"inventory_version"`
}

func (p InstalledPolicyIdentityV1) validFor(b BindingV1) bool {
	return p.NodeID == b.NodeID && p.ProfilePath == ingressinventory.InstalledPolicyPath && digestPattern.MatchString(p.ProfileSHA256) && p.ProfileDevice != 0 && p.ProfileInode != 0 && p.SchemaVersion == 1 && p.InventoryVersion == ingressinventory.ManagedCoverageVersion && strings.HasPrefix(p.NetworkNamespace, "net:[") && strings.HasSuffix(p.NetworkNamespace, "]")
}

func (g *PendingGuard) checkPolicyPendingLocked(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil || g.fd == nil || g.root == nil || g.store == nil {
		return ErrUnverified
	}
	if err := checkAuthorityRoot(g.fd, g.root, g.store.dir, g.store.identity); err != nil {
		return err
	}
	claims, err := loadPending(g.root)
	if err != nil {
		return err
	}
	if current, ok := claims[reservationKey(g.binding)]; !ok || current != g.binding {
		return ErrUnverified
	}
	return nil
}

func (g *PendingGuard) observeInstalledPolicyLocked(ctx context.Context) (InstalledPolicyIdentityV1, error) {
	var zero InstalledPolicyIdentityV1
	if err := g.checkPolicyPendingLocked(ctx); err != nil {
		return zero, err
	}
	p, err := ingressinventory.LoadInstalledManagedNodePolicy(ctx, g.binding.NodeID)
	if err != nil {
		return zero, err
	}
	identity := InstalledPolicyIdentityV1{NodeID: p.NodeID, ProfilePath: p.ProfilePath, ProfileSHA256: p.ProfileSHA256, ProfileDevice: p.ProfileDevice, ProfileInode: p.ProfileInode, NetworkNamespace: p.NetworkNamespace, SchemaVersion: p.SchemaVersion, InventoryVersion: p.InventoryVersion}
	if !identity.validFor(g.binding) {
		return zero, ErrUnverified
	}
	if err := g.checkPolicyPendingLocked(ctx); err != nil {
		return zero, err
	}
	return identity, nil
}

// BindInstalledPolicy pins the independently loaded installed identity under
// the held OS lock and exact pending claim. Drift never rebinds an unresolved
// operation. Even a successful pin leaves operational admission disabled.
func (g *PendingGuard) BindInstalledPolicy(ctx context.Context) (InstalledPolicyIdentityV1, error) {
	var zero InstalledPolicyIdentityV1
	if g == nil {
		return zero, ErrUnverified
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	identity, err := g.observeInstalledPolicyLocked(ctx)
	if err != nil {
		return zero, err
	}
	entries, err := readLifecycle(g.root)
	if err != nil {
		return zero, err
	}
	key := reservationKey(g.binding)
	entry, exists := entries[key]
	if exists && entry.Binding != g.binding {
		return zero, ErrUnverified
	}
	if entry.Policy != nil {
		if *entry.Policy != identity {
			return zero, ErrUnverified
		}
		return identity, nil
	}
	if !exists {
		entry = lifecycleEntry{Binding: g.binding, State: StateNeedsReconciliation}
	}
	entry.Policy = &identity
	entries[key] = entry
	if err := writeLifecycle(g.fd, g.root, entries); err != nil {
		return zero, err
	}
	if err := g.checkPolicyPendingLocked(ctx); err != nil {
		return zero, err
	}
	return identity, nil
}

// CheckInstalledPolicy requires the same held guard, durable claim and pinned
// installed inode/hash/node/namespace on each check. It does not attest that
// every service source is covered and must never be used alone to mint Lease.
func (g *PendingGuard) CheckInstalledPolicy(ctx context.Context) error {
	if g == nil {
		return ErrUnverified
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	identity, err := g.observeInstalledPolicyLocked(ctx)
	if err != nil {
		return err
	}
	entries, err := readLifecycle(g.root)
	if err != nil {
		return err
	}
	entry, ok := entries[reservationKey(g.binding)]
	if !ok || entry.Binding != g.binding || entry.Policy == nil || *entry.Policy != identity {
		return ErrUnverified
	}
	return nil
}
