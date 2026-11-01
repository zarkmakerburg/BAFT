//go:build !linux

package ingressinventory

import "context"

// CheckManagedExecutionScope: operational public ingress is Linux-only.
func CheckManagedExecutionScope(context.Context) error { return ErrCoverageIncomplete }
