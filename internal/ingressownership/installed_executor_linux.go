//go:build linux

package ingressownership

import (
	"context"
	"errors"

	"github.com/zarkmakerburg/baft/internal/ingressinventory"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// NewInstalledManagedLABExecutor discovers only protected local fixed-path
// configuration in an independently measured private Linux LAB. It never
// creates/flushed nft tables or chains and never recovers authority from a
// mutation journal. Normal Agent code may retain it for one process lifetime.
func NewInstalledManagedLABExecutor(ctx context.Context, nodeID string) (*OperationalExecutor, error) {
	pin, err := ingressinventory.PinInstalledManagedLABExecution(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	cfg, err := ingressinventory.LoadInstalledManagedExecutorConfig(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	journal, err := OpenDurableJournal(ingressinventory.InstalledManagedMutationJournalPath)
	if err != nil {
		return nil, err
	}
	recovered, err := journal.Recover()
	if err != nil || len(recovered.NeedsReconciliation) != 0 {
		return nil, errors.Join(ErrUnverified, err)
	}
	runner := installedManagedNFTRunner{config: cfg}
	raw, err := runner.ReadRuleset(ctx)
	if err != nil || verifyNFTLabChain(raw, cfg.ChainHandle) != nil {
		return nil, errors.Join(ErrFirewallUnverified, err)
	}
	// Validate the complete installed ingress rule snapshot too; a malformed
	// or unrelated matching rule is never accepted as an empty trusted chain.
	if _, err := ParseNFTIngressRules(raw); err != nil {
		return nil, err
	}
	if err := ingressinventory.CheckInstalledManagedLABAtEffect(ctx, nodeID, pin); err != nil {
		return nil, err
	}
	if err := ingressinventory.CheckInstalledManagedExecutorConfig(ctx, cfg); err != nil {
		return nil, err
	}
	return NewOperationalExecutor(NewRuntime(), &NFTLabBackend{Runner: runner, ExpectedChainHandle: cfg.ChainHandle},
		&installedManagedJournal{journal: journal, config: cfg})
}

// Fixed executable ownership, inode and bytes are rechecked before exec.
// Privileged host administrators able to replace root-owned paths between
// that check and exec are outside the supported private-LAB trust boundary.
type installedManagedNFTRunner struct {
	config ingressinventory.InstalledManagedExecutorConfig
}

func (r installedManagedNFTRunner) ReadRuleset(ctx context.Context) ([]byte, error) {
	if err := ingressinventory.CheckInstalledManagedExecutorConfig(ctx, r.config); err != nil {
		return nil, err
	}
	return (NFTCommandRunner{Binary: ingressinventory.InstalledManagedNFTBinary}).ReadRuleset(ctx)
}
func (r installedManagedNFTRunner) Run(ctx context.Context, args ...string) error {
	if err := ingressinventory.CheckInstalledManagedExecutorConfig(ctx, r.config); err != nil {
		return err
	}
	return (NFTCommandRunner{Binary: ingressinventory.InstalledManagedNFTBinary}).Run(ctx, args...)
}

type installedManagedJournal struct {
	journal *DurableJournal
	config  ingressinventory.InstalledManagedExecutorConfig
}

func (j *installedManagedJournal) Advance(record MutationRecord) error {
	if err := ingressinventory.CheckInstalledManagedExecutorConfig(context.Background(), j.config); err != nil {
		return err
	}
	return j.journal.Advance(record)
}
func (j *installedManagedJournal) Current(proof ingressmanifest.Proof) (MutationRecord, bool, error) {
	if err := ingressinventory.CheckInstalledManagedExecutorConfig(context.Background(), j.config); err != nil {
		return MutationRecord{}, false, err
	}
	return j.journal.Current(proof)
}
