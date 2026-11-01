//go:build linux

package ingressinventory

import "context"

func measurePinnedLinuxLAB(ctx context.Context, ns, profile string) (PrivateLABExecutionEvidence, error) {
	e, err := measureManagedExecution(ctx, ns, profile)
	if err != nil {
		return PrivateLABExecutionEvidence{}, err
	}
	return observeLinuxExecution(ctx, e)
}
