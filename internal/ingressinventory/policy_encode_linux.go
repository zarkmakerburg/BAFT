//go:build linux

package ingressinventory

import (
	"context"
	"encoding/json"
	"errors"
)

// MarshalInstalledManagedNodePolicyV1 encodes installer configuration using
// the SAME wire type and validation as the fixed installed-policy decoder.
// Metadata is not encoded. Neither these bytes nor a valid profile are an
// inventory-completeness assertion or a live ingress permit.
func MarshalInstalledManagedNodePolicyV1(ctx context.Context, p InstalledManagedNodePolicyV1) ([]byte, error) {
	if ctx == nil {
		return nil, ErrUnsupportedManagedNode
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(ErrUnsupportedManagedNode, err)
	}
	ns, err := namespaceIdentity()
	if err != nil {
		return nil, err
	}
	wire := installedPolicyV1{SchemaVersion: p.SchemaVersion, NodeID: p.NodeID, Platform: p.Platform, NetNS: p.NetworkNamespace, Transport: p.Transport, InventoryV1: p.InventoryVersion, Manager: p.Manager, SourceRoots: append([]string(nil), p.SourceRoots...)}
	raw, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}
	if len(raw) > 32<<10 {
		return nil, ErrUnsupportedManagedNode
	}
	if _, err := parseInstalledPolicy(raw, p.NodeID, ns); err != nil {
		return nil, err
	}
	for _, root := range wire.SourceRoots {
		if _, _, err := protectedSourceRootIdentity(root); err != nil {
			return nil, errors.Join(ErrUnsupportedManagedNode, err)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(ErrUnsupportedManagedNode, err)
	}
	return raw, nil
}
