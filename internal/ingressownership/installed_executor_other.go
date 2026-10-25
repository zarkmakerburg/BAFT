//go:build !linux

package ingressownership

import (
	"context"
	"github.com/zarkmakerburg/baft/internal/ingressinventory"
)

func NewInstalledManagedLABExecutor(context.Context, string) (*OperationalExecutor, error) {
	return nil, ingressinventory.ErrUnsupportedManagedNode
}
