package ingressinventory

import (
	"bytes"
	"context"
	"encoding/json"
)

const InstalledManagedExecutorPath = "/etc/baft/ingress/executor.json"
const InstalledManagedMutationJournalPath = "/var/lib/baft-agent/ingress-mutations"
const InstalledManagedNFTBinary = "/usr/sbin/nft"

// Configuration and observed identities are inert data, never an execution
// permit. The fixed-path loader and concrete held lease are both mandatory.
type InstalledManagedExecutorConfig struct {
	SchemaVersion    int    `json:"schema_version"`
	NodeID           string `json:"node_id"`
	NetworkNamespace string `json:"netns"`
	ChainHandle      uint64 `json:"chain_handle"`
	ProfileDevice    uint64 `json:"-"`
	ProfileInode     uint64 `json:"-"`
	ProfileSHA256    string `json:"-"`
	JournalDevice    uint64 `json:"-"`
	JournalInode     uint64 `json:"-"`
	NFTDevice        uint64 `json:"-"`
	NFTInode         uint64 `json:"-"`
	NFTSHA256        string `json:"-"`
}

func LoadInstalledManagedExecutorConfig(ctx context.Context, nodeID string) (InstalledManagedExecutorConfig, error) {
	return loadInstalledManagedExecutorConfig(ctx, nodeID)
}

func CheckInstalledManagedExecutorConfig(ctx context.Context, pinned InstalledManagedExecutorConfig) error {
	current, err := loadInstalledManagedExecutorConfig(ctx, pinned.NodeID)
	if err != nil {
		return err
	}
	if current != pinned {
		return ErrCoverageIncomplete
	}
	return nil
}

func parseInstalledManagedExecutorConfig(raw []byte, nodeID, netns string) (InstalledManagedExecutorConfig, error) {
	var cfg InstalledManagedExecutorConfig
	if len(raw) == 0 || len(raw) > 32<<10 || json.Unmarshal(raw, &cfg) != nil {
		return cfg, ErrUnsupportedManagedNode
	}
	canonical, err := json.Marshal(cfg)
	if err != nil || !bytes.Equal(canonical, raw) || cfg.SchemaVersion != 1 || nodeID == "" ||
		cfg.NodeID != nodeID || cfg.NetworkNamespace != netns || netns == "" || cfg.ChainHandle == 0 {
		return InstalledManagedExecutorConfig{}, ErrUnsupportedManagedNode
	}
	return cfg, nil
}
