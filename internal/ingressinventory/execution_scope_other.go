//go:build !linux

package ingressinventory

import "context"

// CheckManagedExecutionScope: operational public ingress is Linux-only.
func CheckManagedExecutionScope(context.Context, string) error { return ErrCoverageIncomplete }
