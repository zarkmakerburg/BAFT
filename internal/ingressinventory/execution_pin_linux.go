//go:build linux

package ingressinventory

import (
	"context"
	"errors"
	"fmt"
	"reflect"
)

// pinInstalledManagedLABExecution permits a path override ONLY for
// privileged disposable Linux test fixtures; public callers use the fixed
// root-owned on-node profile. This is inert observation, never a lease.
func pinInstalledManagedLABExecution(ctx context.Context, path, nodeID string) (ManagedLABExecutionPin, error) {
	var zero ManagedLABExecutionPin
	if ctx == nil || ctx.Err() != nil || nodeID == "" {
		return zero, ErrCoverageIncomplete
	}
	policy, err := loadInstalledManagedNodePolicy(ctx, path, nodeID)
	if err != nil {
		return zero, errors.Join(ErrCoverageIncomplete, err)
	}
	first, err := measurePinnedLinuxLAB(ctx, policy.NetworkNamespace)
	if err != nil {
		return zero, err
	}
	source, err := InspectSystemdSocketSources(policy.SourceRoots)
	if err != nil || !source.ScopedComplete {
		return zero, errors.Join(ErrCoverageIncomplete, err)
	}
	fingerprints := make([]SourceIdentity, 0, len(source.Sources))
	for _, observation := range source.Sources {
		fingerprint, err := sourceFingerprint(observation.Root, observation.Claims)
		if err != nil {
			return zero, errors.Join(ErrCoverageIncomplete, err)
		}
		fingerprints = append(fingerprints, fingerprint)
	}
	fresh, err := loadInstalledManagedNodePolicy(ctx, path, nodeID)
	if err != nil || !reflect.DeepEqual(fresh, policy) {
		return zero, ErrCoverageIncomplete
	}
	for i, observation := range source.Sources {
		now, err := sourceFingerprint(observation.Root, observation.Claims)
		if err != nil || !reflect.DeepEqual(now, fingerprints[i]) {
			return zero, ErrCoverageIncomplete
		}
	}
	second, err := measurePinnedLinuxLAB(ctx, policy.NetworkNamespace)
	if err != nil || !first.matches(second) {
		return zero, ErrCoverageIncomplete
	}
	return ManagedLABExecutionPin{
		Policy: policy, Sources: fingerprints,
		Stopped: append([]PersistentTCPPortClaim(nil), source.Claims...),
		LAB: first,
	}, nil
}

func checkInstalledManagedLABAtEffect(ctx context.Context, path, nodeID string, pin ManagedLABExecutionPin) error {
	if ctx == nil || ctx.Err() != nil || nodeID == "" ||
		pin.Policy.NodeID != nodeID || pin.Policy.ProfilePath != path ||
		pin.LAB.ProcessStartTicks == 0 || pin.LAB.ExecutableSHA256 == "" {
		return ErrCoverageIncomplete
	}
	current, err := pinInstalledManagedLABExecution(ctx, path, nodeID)
	if err != nil {
		return errors.Join(ErrCoverageIncomplete, err)
	}
	if !reflect.DeepEqual(pin, current) {
		return fmt.Errorf("%w: managed execution or source identity drift", ErrCoverageIncomplete)
	}
	return nil
}
