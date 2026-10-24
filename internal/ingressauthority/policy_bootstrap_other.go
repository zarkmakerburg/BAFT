//go:build !linux

package ingressauthority

import (
	"context"
	"github.com/zarkmakerburg/baft/internal/ingressinventory"
)

func ProvisionInstalledManagedNodePolicy(ctx context.Context, _ string) (ingressinventory.InstalledManagedNodePolicyV1, error) {
	if ctx != nil && ctx.Err() != nil {
		return ingressinventory.InstalledManagedNodePolicyV1{}, ctx.Err()
	}
	return ingressinventory.InstalledManagedNodePolicyV1{}, ingressinventory.ErrUnsupportedManagedNode
}
