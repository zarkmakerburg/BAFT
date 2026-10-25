//go:build linux

package ingressownership

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"strconv"
	"time"

	"encoding/json"
	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"github.com/zarkmakerburg/baft/internal/release"
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
	for _, root := range []string{"/etc/baft", "/etc/baft-agent", "/var/lib/baft-agent"} {
		if !bytes.Contains(mounts, []byte(" "+root+" ")) {
			t.Fatal("fixed test root is not a private mountpoint")
		}
		raw, err := os.ReadFile(root + "/.executor-bootstrap-lab")
		if err != nil || string(raw) != "disposable-executor-bootstrap-lab" {
			t.Fatal("missing disposable mount identity")
		}
		local, localErr := os.Stat(root)
		host, hostErr := os.Stat("/proc/1/root" + root)
		if localErr != nil || hostErr == nil && os.SameFile(local, host) {
			t.Fatal("host path exposed at fixed test root")
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
	exerciseInstalledSignedLifecycle(t, x, nodeID)
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

func exerciseInstalledSignedLifecycle(t *testing.T, x *OperationalExecutor, nodeID string) {
	t.Helper()
	ctx := context.Background()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("/etc/baft-agent/bcc-job.pub", []byte(release.EncodePublic(public)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(ingressauthority.InstalledAuthorityDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	backend, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	go func() {
		for {
			c, err := backend.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
		}
	}()
	spec := testSpec(t, "installed-real-signed")
	spec.InstanceID = nodeID
	spec.Config.Target = backend.Addr().String()
	spec.Config.MaxClients, spec.Config.DialTimeoutMS = 128, 3000
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	_, portText, err := net.SplitHostPort(spec.Config.Listen)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	cmd := executorCommandV1(t)
	cmd.Binding.Spec, cmd.Binding.Proof = spec, proof
	cmd.Binding.NodeID, cmd.Binding.Port, cmd.Binding.ConfigPort = nodeID, uint16(port), uint16(port)
	cmd.Binding.ListenEndpoint, cmd.Binding.LocalTarget = spec.Config.Listen, spec.Config.Target
	cmd.JobID = "installed-real-apply"
	cmd.IssuedAt = time.Now().UTC().Truncate(time.Second).Add(-time.Second)
	cmd.ExpiresAt = cmd.IssuedAt.Add(5 * time.Minute)
	store, err := ingressauthority.OpenInstalledManagedReservationStore(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReservePending(cmd.Binding); err != nil {
		t.Fatal(err)
	}
	guard, err := store.HoldPending(cmd.Binding)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	if _, err := guard.BindInstalledPolicy(ctx); err != nil {
		t.Fatal(err)
	}
	if err := guard.MarkApplying(ctx, cmd.Binding); err != nil {
		t.Fatal(err)
	}
	mint := func(c ingressauthority.CommandV1) *ingressauthority.Lease {
		t.Helper()
		if err := guard.RecordCommandIntent(ctx, c); err != nil {
			t.Fatal(err)
		}
		job, err := c.AgentJob()
		if err != nil {
			t.Fatal(err)
		}
		env, err := agentjob.Sign(private, job)
		if err != nil {
			t.Fatal(err)
		}
		lease, err := guard.MintSignedManagedLABLease(ctx, env)
		if err != nil {
			t.Fatal(err)
		}
		return lease
	}
	applyLease := mint(cmd)
	defer applyLease.Close()
	observed, err := x.Apply(ctx, cmd, applyLease)
	if err != nil {
		t.Fatalf("real signed held Apply: %v", err)
	}
	if observed.Listener.SocketIdentity == "" || observed.Firewall.RuleHandle == 0 {
		t.Fatal("missing independent real resource readback")
	}
	conn, err := net.DialTimeout("tcp4", spec.Config.Listen, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	payload := []byte{0, 1, 2, 255, 128, 42}
	if _, err := conn.Write(payload); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, reply); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if !bytes.Equal(payload, reply) {
		t.Fatal("real listener corrupted TCP payload")
	}
	if err := applyLease.Close(); err != nil {
		t.Fatal(err)
	}
	retire := cmd
	retire.Action = "public_ingress_retire"
	retire.JobID = "installed-real-retire"
	retireLease := mint(retire)
	defer retireLease.Close()
	retired, err := x.Retire(ctx, retire, retireLease)
	if err != nil {
		t.Fatalf("real signed held Retire: %v", err)
	}
	if retired.Phase != PhaseRetired || retired.Firewall.RuleHandle != observed.Firewall.RuleHandle ||
		retired.Firewall.ChainHandle != observed.Firewall.ChainHandle || retired.Firewall.OwnerTag != observed.Firewall.OwnerTag {
		t.Fatal("retire lost exact historical kernel identities")
	}
	plan, err := PlanIngressRule(uint16(port), proof)
	if err != nil {
		t.Fatal(err)
	}
	if err := x.transaction.Firewall.verifyAbsent(ctx, plan, observed.Firewall.RuleHandle); err != nil {
		t.Fatalf("removed nft rule absence not independently verified: %v", err)
	}
	if _, err := net.DialTimeout("tcp4", spec.Config.Listen, 100*time.Millisecond); err == nil {
		t.Fatal("retired listener accepts connections")
	}
	t.Log("real signed held Apply/TCP/Retire with durable journal and exact kernel handles PASS")
}
