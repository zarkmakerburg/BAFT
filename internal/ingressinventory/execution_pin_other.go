//go:build !linux

package ingressinventory

import "context"

func pinInstalledManagedLABExecution(context.Context, string, string) (ManagedLABExecutionPin, error) {
	return ManagedLABExecutionPin{}, ErrCoverageIncomplete
}

func checkInstalledManagedLABAtEffect(context.Context, string, string, ManagedLABExecutionPin) error {
	return ErrCoverageIncomplete
}
