//go:build !linux

package ingressauthority

import (
	"context"
	"errors"
	"github.com/zarkmakerburg/baft/internal/ingressinventory"
)

func ProvisionInstalledManagedNodePolicy(ctx context.Context, _ string) (ingressinventory.InstalledManagedNodePolicyV1, error) {
	if ctx != nil && ctx.Err() != nil {
		return ingressinventory.InstalledManagedNodePolicyV1{}, ctx.Err()
	}
	return ingressinventory.InstalledManagedNodePolicyV1{}, ingressinventory.ErrUnsupportedManagedNode
}

// ErrPolicyProfileConflict: see the Linux implementation.
var ErrPolicyProfileConflict = errors.New("a different managed ingress policy is already installed")

func ProvisionInstalledManagedNodePolicyProfile(ctx context.Context, nodeID, _ string) (ingressinventory.InstalledManagedNodePolicyV1, error) {
	return ProvisionInstalledManagedNodePolicy(ctx, nodeID)
}
