//go:build linux

package ingressownership

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressinventory"
)

// Only the launcher that privately masks /etc and /var/lib may run this test.
// A sentinel by itself is insufficient: both fixed roots must be mountpoints
// and this process must be outside init's mount and network namespaces.
func requireExecutorBootstrapLAB(t *testing.T) {
	t.Helper()
	if os.Getenv("BAFT_EXECUTOR_BOOTSTRAP_LAB") != "1" || os.Geteuid() != 0 {
		t.Skip("requires explicit private mounted Linux LAB")
	}
	for _, kind := range []string{"mnt", "net"} {
		own, e := os.Readlink("/proc/self/ns/" + kind)
		init, f := os.Readlink("/proc/1/ns/" + kind)
		if e != nil || f != nil || own == init {
			t.Fatal("not independently isolated from init: " + kind)
		}
	}
	mounts, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{"/etc/baft", "/var/lib/baft-agent"} {
		if !bytes.Contains(mounts, []byte(" "+root+" ")) {
			t.Fatal("fixed test root is not a private mountpoint")
		}
		raw, err := os.ReadFile(root + "/.executor-bootstrap-lab")
		if err != nil || string(raw) != "disposable-executor-bootstrap-lab" {
			t.Fatal("missing disposable mount identity")
		}
	}
}

func TestInstalledManagedExecutorBootstrapLAB(t *testing.T) {
	requireExecutorBootstrapLAB(t)
	ctx := context.Background()
	const nodeID = "ir-executor-lab"
	ns, err := os.Readlink("/proc/self/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("/etc/baft/ingress/sources", 0700); err != nil {
		t.Fatal(err)
	}
	policy, err := ingressinventory.MarshalInstalledManagedNodePolicyV1(ctx, ingressinventory.InstalledManagedNodePolicyV1{
		SchemaVersion: 1, InventoryVersion: 1, NodeID: nodeID, Platform: "linux", NetworkNamespace: ns, Transport: "dedicated",
		Manager: "systemd-socket-v1", SourceRoots: []string{"/etc/baft/ingress/sources"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ingressinventory.InstalledPolicyPath, policy, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(ingressinventory.InstalledManagedMutationJournalPath, 0700); err != nil {
		t.Fatal(err)
	}
	nft := func(args ...string) []byte {
		t.Helper()
		out, err := exec.Command(ingressinventory.InstalledManagedNFTBinary, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("isolated fixture nft %v: %s %v", args, out, err)
		}
		return out
	}
	nft("add", "table", "inet", "baft")
	defer nft("delete", "table", "inet", "baft")
	nft("add", "chain", "inet", "baft", "ingress", "{ type filter hook input priority -100; policy accept; }")
	var rules struct {
		NFTables []struct {
			Chain *struct {
				Name   string `json:"name"`
				Handle uint64 `json:"handle"`
			} `json:"chain"`
		} `json:"nftables"`
	}
	if err := json.Unmarshal(nft("-j", "-a", "list", "ruleset"), &rules); err != nil {
		t.Fatal(err)
	}
	var handle uint64
	for _, r := range rules.NFTables {
		if r.Chain != nil && r.Chain.Name == "ingress" {
			handle = r.Chain.Handle
		}
	}
	if handle == 0 {
		t.Fatal("missing actual kernel chain handle")
	}
	cfg := ingressinventory.InstalledManagedExecutorConfig{SchemaVersion: 1, NodeID: nodeID, NetworkNamespace: ns, ChainHandle: handle}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ingressinventory.InstalledManagedExecutorPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	x, err := NewInstalledManagedLABExecutor(ctx, nodeID)
	if err != nil {
		t.Fatalf("actual installed bootstrap refused: %v", err)
	}
	if x == nil {
		t.Fatal("missing executor")
	}
	cmd := executorCommandV1(t)
	before := nft("-j", "-a", "list", "ruleset")
	if _, err := x.Apply(ctx, cmd, nil); err == nil {
		t.Fatal("bootstrap manufactured a live lease")
	}
	after := nft("-j", "-a", "list", "ruleset")
	if !bytes.Equal(before, after) {
		t.Fatal("unleased bootstrap mutated nft")
	}
	pinned, err := ingressinventory.LoadInstalledManagedExecutorConfig(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	for _, fault := range []string{"config-inode", "config-mode", "chain", "journal-inode", "journal-mode"} {
		t.Run(fault, func(t *testing.T) {
			switch fault {
			case "config-inode":
				path := ingressinventory.InstalledManagedExecutorPath
				if err := os.Rename(path, path+".old"); err != nil {
					t.Fatal(err)
				}
				defer os.Rename(path+".old", path)
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
				defer os.Remove(path)
			case "config-mode":
				if err := os.Chmod(ingressinventory.InstalledManagedExecutorPath, 0644); err != nil {
					t.Fatal(err)
				}
				defer os.Chmod(ingressinventory.InstalledManagedExecutorPath, 0600)
			case "chain":
				nft("delete", "chain", "inet", "baft", "ingress")
				defer nft("add", "chain", "inet", "baft", "ingress", "{ type filter hook input priority -100; policy accept; }")
			case "journal-inode":
				path := ingressinventory.InstalledManagedMutationJournalPath
				if err := os.Rename(path, path+".old"); err != nil {
					t.Fatal(err)
				}
				defer os.Rename(path+".old", path)
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
				defer os.Remove(path)
			case "journal-mode":
				if err := os.Chmod(ingressinventory.InstalledManagedMutationJournalPath, 0755); err != nil {
					t.Fatal(err)
				}
				defer os.Chmod(ingressinventory.InstalledManagedMutationJournalPath, 0700)
			}
			if _, err := x.transaction.Firewall.Runner.ReadRuleset(ctx); fault != "chain" && err == nil {
				t.Fatal("installed config identity drift accepted")
			}
			if fault == "chain" {
				_, err := x.transaction.Firewall.inspect(ctx)
				if err == nil {
					t.Fatal("missing pinned kernel chain accepted")
				}
			} else if err := ingressinventory.CheckInstalledManagedExecutorConfig(ctx, pinned); err == nil {
				t.Fatal("pinned provisioning drift ignored")
			}
		})
	}
}
