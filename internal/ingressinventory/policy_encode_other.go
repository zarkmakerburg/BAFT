//go:build !linux

package ingressinventory

import "context"

func MarshalInstalledManagedNodePolicyV1(ctx context.Context, _ InstalledManagedNodePolicyV1) ([]byte, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return nil, ErrUnsupportedManagedNode
}
