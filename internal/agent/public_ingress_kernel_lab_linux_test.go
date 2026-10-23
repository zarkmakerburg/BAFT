//go:build linux

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"github.com/zarkmakerburg/baft/internal/ingressownership"
	"github.com/zarkmakerburg/baft/internal/release"
)

// Fail before the real delete, leaving a rule that must be independently
// reverified on retry. All other operations use the actual kernel backend.
type kernelRetireFault struct {
	ingressownership.NFTCommandRunner
	failDelete bool
}

type kernelJournalFault struct {
	ingressownership.MutationJournal
	failRetired bool
}

func (j *kernelJournalFault) Advance(r ingressownership.MutationRecord) error {
	if j.failRetired && r.Phase == ingressownership.PhaseRetired {
		return errors.New("LAB injected final journal failure after kernel delete")
	}
	return j.MutationJournal.Advance(r)
}

func (r *kernelRetireFault) Run(ctx context.Context, args ...string) error {
	if r.failDelete && len(args) > 0 && args[0] == "delete" {
		return errors.New("LAB injected failure before kernel delete")
	}
	return r.NFTCommandRunner.Run(ctx, args...)
}

// This test mutates only a disposable network namespace. It deliberately uses
// the real signature verifier, Agent dispatcher, journal, listener and nft binary.
func TestPublicIngressLABKernelSignedLifecycle(t *testing.T) {
	if os.Getenv("BAFT_NFT_KERNEL_LAB") != "1" {
		t.Skip("dedicated Linux network namespace only")
	}
	self, err := os.Readlink("/proc/self/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	init, err := os.Readlink("/proc/1/ns/net")
	if err != nil || self == init || os.Geteuid() != 0 {
		t.Fatal("requires root in a separate network namespace")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const binary = "/usr/sbin/nft"
	run := func(args ...string) []byte {
		t.Helper()
		raw, err := exec.CommandContext(ctx, binary, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("nft %v: %v: %s", args, err, raw)
		}
		return raw
	}
	run("add", "table", "inet", "baft")
	defer exec.Command(binary, "delete", "table", "inet", "baft").Run()
	run("add", "chain", "inet", "baft", "ingress", "{ type filter hook input priority -100; policy accept; }")
	run("add", "table", "inet", "agent_keep")
	defer exec.Command(binary, "delete", "table", "inet", "agent_keep").Run()
	run("add", "chain", "inet", "agent_keep", "keep")
	run("add", "rule", "inet", "agent_keep", "keep", "tcp", "dport", "2053", "accept")
	baseline := run("-j", "list", "table", "inet", "agent_keep")
	var chain struct {
		NFTables []struct {
			Chain struct {
				Handle uint64 `json:"handle"`
			} `json:"chain"`
		} `json:"nftables"`
	}
	if err := json.Unmarshal(run("-j", "list", "chain", "inet", "baft", "ingress"), &chain); err != nil {
		t.Fatal(err)
	}
	var handle uint64
	for _, item := range chain.NFTables {
		if item.Chain.Handle != 0 {
			handle = item.Chain.Handle
		}
	}
	if handle == 0 {
		t.Fatal("missing kernel chain identity")
	}

	target, err := net.Listen("tcp4", "127.0.0.1:1443")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	echoDone := make(chan struct{})
	go func() {
		defer close(echoDone)
		for {
			c, err := target.Accept()
			if err != nil {
				return
			}
			_ = c.SetDeadline(time.Now().Add(3 * time.Second))
			_, _ = io.Copy(c, c)
			_ = c.Close()
		}
	}()
	defer func() { _ = target.Close(); <-echoDone }()
	// A real unrelated service must survive Apply and Retire.
	keep, err := net.Listen("tcp4", "127.0.0.1:2053")
	if err != nil {
		t.Fatal(err)
	}
	defer keep.Close()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	journalDir := filepath.Join(dir, "journal")
	if err := os.Mkdir(journalDir, 0700); err != nil {
		t.Fatal(err)
	}
	journal, err := ingressownership.OpenDurableJournal(journalDir)
	if err != nil {
		t.Fatal(err)
	}
	runner := &kernelRetireFault{NFTCommandRunner: ingressownership.NFTCommandRunner{Binary: binary}}
	backend := &ingressownership.NFTLabBackend{Runner: runner, ExpectedChainHandle: handle}
	journalFault := &kernelJournalFault{MutationJournal: journal}
	tx := &ingressownership.NFTLabTransaction{Runtime: ingressownership.NewRuntime(), Firewall: backend, Journal: journalFault}
	manifestDir := filepath.Join(dir, "manifest")
	if err := os.Mkdir(manifestDir, 0700); err != nil {
		t.Fatal(err)
	}
	a := &Agent{cfg: Config{NodeID: "ir-a", IngressManifestDir: manifestDir, IngressLABExecutor: tx}}
	pub, key, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	j, proof := prepareJob(t)
	verify := func(job agentjob.Job) agentjob.Job {
		t.Helper()
		env, err := agentjob.Sign(key, job)
		if err != nil {
			t.Fatal(err)
		}
		verified, err := (agentjob.Verifier{BCCKey: pub, NodeID: job.NodeID}).Verify(env, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		return verified
	}
	execute := func(job agentjob.Job) string {
		t.Helper()
		_, output, err := a.executeFull(ctx, verify(job))
		if err != nil {
			t.Fatalf("%s: %v", job.Action, err)
		}
		return output
	}
	execute(j)
	j.Action, j.JobID = agentjob.ActionPublicIngressApply, "kernel-apply"
	var receipt publicIngressLABReceipt
	if err := json.Unmarshal([]byte(execute(j)), &receipt); err != nil {
		t.Fatal(err)
	}
	if !receipt.LiveOwnershipVerified || receipt.Proof != proof {
		t.Fatalf("invalid apply receipt: %+v", receipt)
	}
	spec := ingressmanifest.Spec{TunnelID: "tun-a", Generation: 1, Config: ingressmanifest.Config{Listen: "0.0.0.0:54619", Target: "127.0.0.1:1443", MaxClients: 128, DialTimeoutMS: 3000}}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = tx.Retire(cleanupCtx, spec, proof)
	}()
	execute(j) // Same owner is idempotent with the actual kernel handle.
	conn, err := net.DialTimeout("tcp4", "127.0.0.1:54619", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	payload := bytes.Repeat([]byte("signed-kernel-ingress"), 256)
	if _, err := conn.Write(payload); err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	got := make([]byte, len(payload))
	_, err = io.ReadFull(conn, got)
	_ = conn.Close()
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("port 54619 traffic: %v", err)
	}
	plan, err := ingressownership.PlanIngressRule(54619, proof)
	if err != nil {
		t.Fatal(err)
	}
	if observed, err := backend.Inspect(ctx, plan); err != nil || observed.Handle == 0 {
		t.Fatalf("kernel ownership: %+v %v", observed, err)
	}
	j.Action, j.JobID = agentjob.ActionPublicIngressRetire, "kernel-retire"
	runner.failDelete = true
	if _, output, err := a.executeFull(ctx, verify(j)); err == nil || output != "" {
		t.Fatalf("failed kernel retirement claimed success: %q %v", output, err)
	}
	if _, err := backend.Inspect(ctx, plan); err != nil {
		t.Fatalf("failed deletion lost exact kernel ownership: %v", err)
	}
	foreign, err := net.Listen("tcp4", "0.0.0.0:54619")
	if err != nil {
		t.Fatalf("failed delete did not stop original listener: %v", err)
	}
	defer foreign.Close()
	runner.failDelete = false
	journalFault.failRetired = true
	if _, output, err := a.executeFull(ctx, verify(j)); err == nil || output != "" {
		t.Fatalf("failed final journal write claimed success: %q %v", output, err)
	}
	rawRules := run("-j", "list", "chain", "inet", "baft", "ingress")
	if bytes.Contains(rawRules, []byte(plan.Tag)) {
		t.Fatal("successful deletion left owned rule")
	}
	current, found, err := journal.Current(proof)
	if err != nil || !found || current.Phase != ingressownership.PhaseRetireIntent {
		t.Fatalf("lost final journal uncertainty: %+v %v", current, err)
	}
	journalFault.failRetired = false
	if err := json.Unmarshal([]byte(execute(j)), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.LiveOwnershipVerified {
		t.Fatal("retired receipt claims live ownership")
	}
	foreignClient, err := net.DialTimeout("tcp4", "127.0.0.1:54619", time.Second)
	if err != nil {
		t.Fatalf("retirement retry closed unrelated replacement listener: %v", err)
	}
	_ = foreignClient.Close()
	_ = foreign.Close()
	rebound, err := net.Listen("tcp4", "0.0.0.0:54619")
	if err != nil {
		t.Fatalf("retire leaked socket: %v", err)
	}
	_ = rebound.Close()
	if _, err := backend.Inspect(ctx, plan); err == nil {
		t.Fatal("retire leaked kernel rule")
	}
	recovered, err := journal.Recover()
	if err != nil || len(recovered.Records) != 1 || recovered.Records[0].Phase != ingressownership.PhaseRetired || len(recovered.NeedsReconciliation) != 0 {
		t.Fatalf("retired journal: %+v %v", recovered, err)
	}
	if !bytes.Equal(baseline, run("-j", "list", "table", "inet", "agent_keep")) {
		t.Fatal("unrelated 2053 rule changed")
	}
	c, err := net.DialTimeout("tcp4", keep.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("unrelated 2053 service lost: %v", err)
	}
	_ = c.Close()
}
