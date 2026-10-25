package ingressownership

import (
	"context"
	"errors"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressinventory"
)

// Called under the transaction mutex immediately before each operational
// journal, listener and nft effect. Source/process pins alone cannot detect
// a new foreign socket in either address family. This is an additional veto,
// never permission to mutate without the concrete held lease.
func (t *NFTLabTransaction) checkOperationalEffectPort(ctx context.Context, b ingressauthority.BindingV1) error {
	observed, scanErr := ingressinventory.CollectInstalledManagedTCP(ctx, b.NodeID, b.Port)
	if scanErr != nil && !errors.Is(scanErr, ingressinventory.ErrClaimConflict) {
		return scanErr
	}
	if observed.Port != b.Port || !observed.CoverageComplete || len(observed.UnknownSources) != 0 {
		return ingressinventory.ErrCoverageIncomplete
	}
	t.Runtime.mu.Lock()
	session, exists := t.Runtime.sessions[b.ListenEndpoint]
	t.Runtime.mu.Unlock()
	if exists {
		if session.proof != b.Proof {
			return ErrConflict
		}
		return (&OperationalExecutor{transaction: t}).ownedLiveListener(ctx, b, observed)
	}
	if scanErr != nil {
		return scanErr
	}
	return retiredPortObservation(b, observed)
}
