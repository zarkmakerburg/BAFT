//go:build !linux

package ingressinventory

import "context"

func collectFromInstalledPath(_ context.Context, path, nodeID string, port uint16) (CoverageEvidence, error) {
	return coverageError(CoverageEvidence{NodeID: nodeID, Port: port, PolicyPath: path}, "Linux managed node required")
}

func loadInstalledManagedNodePolicy(_ context.Context, _ string, _ string) (InstalledManagedNodePolicyV1, error) {
	return InstalledManagedNodePolicyV1{}, ErrUnsupportedManagedNode
}
