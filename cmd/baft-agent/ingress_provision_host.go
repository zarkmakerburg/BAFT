package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressinventory"
	"github.com/zarkmakerburg/baft/internal/ingressownership"
)

// privateSubdir returns stateDir/name, creating it 0700 if absent and refusing
// symlinks, non-directories, foreign owners or any group/other access.
func privateSubdir(stateDir, name string) (string, error) {
	if !filepath.IsAbs(stateDir) || filepath.Clean(stateDir) != stateDir {
		return "", errors.New("state dir must be a clean absolute path")
	}
	dir := filepath.Join(stateDir, name)
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode().Perm() != 0o700 || int(st.Uid) != os.Geteuid() {
		return "", fmt.Errorf("%s must be a private 0700 directory owned by the agent user", dir)
	}
	return dir, nil
}

func hostIngressDeps(logf func(string, ...any)) ingressDeps {
	return ingressDeps{
		LoadPolicy: func(ctx context.Context, nodeID string) error {
			_, err := ingressinventory.LoadInstalledManagedNodePolicy(ctx, nodeID)
			return err
		},
		OpenStore: func(stateDir string) (*ingressauthority.ReservationStore, error) {
			dir, err := privateSubdir(stateDir, "ingress-authority")
			if err != nil {
				return nil, err
			}
			return ingressauthority.OpenReservationStore(dir)
		},
		OpenJournal: func(stateDir string) (ingressownership.MutationJournal, error) {
			dir, err := privateSubdir(stateDir, "ingress-journal")
			if err != nil {
				return nil, err
			}
			return ingressownership.OpenDurableJournal(dir)
		},
		RecoverJournal: func(_ context.Context, stateDir string) (int, error) {
			dir, err := privateSubdir(stateDir, "ingress-journal")
			if err != nil {
				return 0, err
			}
			j, err := ingressownership.OpenDurableJournal(dir)
			if err != nil {
				return 0, err
			}
			rec, err := j.Recover()
			return len(rec.NeedsReconciliation), err
		},
		ChainHandle: hostChainHandle,
		Logf:        logf,
	}
}

// hostChainHandle finds the pre-provisioned inet/baft/ingress chain with the
// pinned base-chain layout and requires the same nonzero handle on two reads.
// The agent's nft runner cannot create tables or chains, so an absent or
// differently laid out chain is a refusal, never something to repair.
func hostChainHandle(ctx context.Context) (uint64, ingressownership.NFTMutationRunner, error) {
	var bin string
	for _, c := range []string{"/usr/sbin/nft", "/sbin/nft"} {
		if st, err := os.Stat(c); err == nil && st.Mode().IsRegular() {
			bin = c
			break
		}
	}
	if bin == "" {
		return 0, nil, errors.New("nft binary not found")
	}
	runner := ingressownership.NFTCommandRunner{Binary: bin}
	handle, err := stableChainHandle(ctx, runner)
	if err != nil {
		return 0, nil, err
	}
	return handle, runner, nil
}

func stableChainHandle(ctx context.Context, r ingressownership.NFTRunner) (uint64, error) {
	var first uint64
	for i := 0; i < 2; i++ {
		raw, err := r.ReadRuleset(ctx)
		if err != nil {
			return 0, err
		}
		h, err := ingressownership.ReadNFTLabChainHandle(raw)
		if err != nil {
			return 0, err
		}
		if i == 0 {
			first = h
		} else if h != first {
			return 0, errors.New("nft chain handle not stable")
		}
	}
	return first, nil
}
