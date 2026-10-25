//go:build !linux

package ingressinventory

import "context"

func loadInstalledManagedExecutorConfig(context.Context, string) (InstalledManagedExecutorConfig, error) {
	return InstalledManagedExecutorConfig{}, ErrUnsupportedManagedNode
}
