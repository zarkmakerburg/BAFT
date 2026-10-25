package ingressinventory

import (
	"encoding/json"
	"testing"
)

func TestInstalledExecutorConfigRejectsAmbiguousOrForeignProvisioning(t *testing.T) {
	cfg := InstalledManagedExecutorConfig{SchemaVersion: 1, NodeID: "ir-test", NetworkNamespace: "net:[123]", ChainHandle: 7}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseInstalledManagedExecutorConfig(raw, cfg.NodeID, cfg.NetworkNamespace); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string]string{
		"duplicate":   `{"schema_version":1,"node_id":"ir-test","netns":"net:[123]","chain_handle":7,"chain_handle":7}`,
		"case":        `{"schema_version":1,"Node_ID":"ir-test","netns":"net:[123]","chain_handle":7}`,
		"extra":       `{"schema_version":1,"node_id":"ir-test","netns":"net:[123]","chain_handle":7,"permit":true}`,
		"metadata":    `{"schema_version":1,"node_id":"ir-test","netns":"net:[123]","chain_handle":7,"NFTInode":123}`,
		"null":        "null",
		"zero":        `{"schema_version":1,"node_id":"ir-test","netns":"net:[123]","chain_handle":0}`,
		"wrong-node":  `{"schema_version":1,"node_id":"foreign","netns":"net:[123]","chain_handle":7}`,
		"wrong-netns": `{"schema_version":1,"node_id":"ir-test","netns":"net:[999]","chain_handle":7}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseInstalledManagedExecutorConfig([]byte(bad), cfg.NodeID, cfg.NetworkNamespace); err == nil {
				t.Fatal("ambiguous/foreign executor configuration accepted")
			}
		})
	}
}
