//go:build linux

package ingressinventory

import "context"

func measurePinnedLinuxLAB(ctx context.Context, ns string) (PrivateLABExecutionEvidence, error) {
	e, err := measurePrivateLinuxLAB(ctx, ns)
	if err != nil {
		return PrivateLABExecutionEvidence{}, err
	}
	return observeLinuxExecution(ctx, e)
}
