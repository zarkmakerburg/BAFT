package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/zarkmakerburg/baft/internal/agent"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressownership"
)

// ingressDeps are the host-touching steps of operational-ingress provisioning,
// injectable so the fail-closed decision is testable without root or nft.
type ingressDeps struct {
	// LoadPolicy loads and verifies the installed managed node policy for nodeID.
	LoadPolicy func(ctx context.Context, nodeID string) error
	// OpenStore opens the private reservation store (0700, owned by euid).
	OpenStore func(stateDir string) (*ingressauthority.ReservationStore, error)
	// OpenJournal opens the private durable mutation journal.
	OpenJournal func(stateDir string) (ingressownership.MutationJournal, error)
	// RecoverJournal validates the whole journal chain without mutating it and
	// returns how many entries need reconciliation.
	RecoverJournal func(ctx context.Context, stateDir string) (int, error)
	// ChainHandle reads the pre-provisioned owned nft table+chain and returns a
	// stable (confirmed on two reads) nonzero chain handle plus the runner.
	ChainHandle func(ctx context.Context) (uint64, ingressownership.NFTMutationRunner, error)
	// Logf receives diagnostics; may be nil.
	Logf func(format string, args ...any)
}

var errUnavailable = errors.New("resource unavailable")

// buildIngressOperational returns a non-nil *agent.IngressOperational only when
// EVERY prerequisite holds. Any failure returns nil and a reason: the agent then
// advertises no capability and v1 jobs stay BLOCKED. It never selects the LAB
// executor or an in-memory firewall/journal.
func buildIngressOperational(ctx context.Context, nodeID, stateDir string, d ingressDeps) (*agent.IngressOperational, error) {
	logf := d.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	if ctx == nil || nodeID == "" || stateDir == "" || d.LoadPolicy == nil || d.OpenStore == nil ||
		d.OpenJournal == nil || d.RecoverJournal == nil || d.ChainHandle == nil {
		return nil, errors.New("ingress provisioning is not fully wired")
	}
	if err := d.LoadPolicy(ctx, nodeID); err != nil {
		return nil, fmt.Errorf("installed managed node policy: %w", err)
	}
	store, err := d.OpenStore(stateDir)
	if err == nil && store == nil {
		err = errUnavailable
	}
	if err != nil {
		return nil, fmt.Errorf("reservation store: %w", err)
	}
	// Restart behaviour: leftovers stay fenced in the store/journal. We only
	// read and report them; no cleanup, release or lease is inferred.
	leftovers, err := store.InspectFencedLeftovers()
	if err != nil {
		return nil, fmt.Errorf("reservation lifecycle scan: %w", err)
	}
	for _, l := range leftovers {
		logf("baft-agent: ingress port %d remains fenced (state %q); operator reconciliation required", l.Binding.Port, l.State)
	}
	journal, err := d.OpenJournal(stateDir)
	if err == nil && journal == nil {
		err = errUnavailable
	}
	if err != nil {
		return nil, fmt.Errorf("mutation journal: %w", err)
	}
	n, err := d.RecoverJournal(ctx, stateDir)
	if err != nil {
		return nil, fmt.Errorf("mutation journal recovery scan: %w", err)
	}
	if n > 0 {
		logf("baft-agent: %d journal entr(ies) need reconciliation and remain fenced", n)
	}
	handle, runner, err := d.ChainHandle(ctx)
	if err == nil && (handle == 0 || runner == nil) {
		err = errUnavailable
	}
	if err != nil {
		return nil, fmt.Errorf("nft owned chain: %w", err)
	}
	nft := &ingressownership.NFTLabBackend{Runner: runner, ExpectedChainHandle: handle}
	exec, err := ingressownership.NewOperationalExecutor(ingressownership.NewRuntime(), nft, journal)
	if err == nil && exec == nil {
		err = errUnavailable
	}
	if err != nil {
		return nil, fmt.Errorf("operational executor: %w", err)
	}
	return &agent.IngressOperational{Store: store, Executor: exec}, nil
}
