//go:build linux

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/bcc"
	"github.com/zarkmakerburg/baft/internal/config"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressownership"
	"github.com/zarkmakerburg/baft/internal/node"
	"github.com/zarkmakerburg/baft/tests/e2e/operational/oplab"
)

// runtimeServices replaces systemd supervision with actual node.Runtime.Run
// goroutines. This is explicit LAB supervision, not process/systemd acceptance.
// It leaves the signed BCC/Agent, binary pairing/validation, Noise/TLS transport,
// socket inventory, ingress listener, nft, journal and HTTPS receipts real.
// The installed private-netns inventory requires a sole process at admission.
type runtimeServices struct {
	t        testing.TB
	mu       sync.Mutex
	units    string
	services map[string]*runtimeService
	enabled  map[string]bool
}

type runtimeService struct {
	runtime *node.Runtime
	file    string
	stop    context.CancelFunc
	done    chan struct{}
}

func (s *runtimeServices) stop(name string) error {
	p := s.services[name]
	if p == nil {
		return nil
	}
	p.stop()
	select {
	case <-p.done:
		delete(s.services, name)
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("runtime %s did not stop", name)
	}
}

func (s *runtimeServices) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for name := range s.services {
		if err := s.stop(name); err != nil {
			s.t.Errorf("runtime supervisor cleanup: %v", err)
		}
	}
}

func (s *runtimeServices) identity(name string) any {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p := s.services[name]; p != nil {
		select {
		case <-p.done:
		default:
			return p.runtime // Runtime identity, never a claimed process PID.
		}
	}
	return nil
}

func (s *runtimeServices) Run(ctx context.Context, binary string, args ...string) (string, error) {
	raw, err := exec.CommandContext(ctx, binary, args...).CombinedOutput()
	return strings.TrimSpace(string(raw)), err
}

func (s *runtimeServices) Systemctl(_ context.Context, args ...string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(args) == 1 && args[0] == "daemon-reload" {
		return "", nil
	}
	if len(args) < 2 {
		return "", fmt.Errorf("missing runtime service action/name")
	}
	name := strings.TrimSuffix(args[1], ".service")
	p := s.services[name]
	alive := false
	if p != nil {
		select {
		case <-p.done:
		default:
			alive = true
		}
	}
	switch args[0] {
	case "enable":
		s.enabled[name] = true
	case "disable":
		s.enabled[name] = false
	case "is-enabled":
		if s.enabled[name] {
			return "enabled", nil
		}
		return "disabled", nil
	case "is-active":
		if alive {
			return "active", nil
		}
		return "inactive", nil
	case "show":
		if alive {
			return "0", nil
		}
		return "1", nil
	case "stop":
		return "", s.stop(name)
	case "reload":
		if !alive {
			return "", fmt.Errorf("cannot reload stopped runtime %s", name)
		}
		cfg, err := config.LoadFile(p.file)
		if err != nil {
			return "", err
		}
		_, err = p.runtime.ReloadListener(cfg)
		return "", err
	case "restart", "start":
		if err := s.stop(name); err != nil {
			return "", err
		}
		raw, err := os.ReadFile(filepath.Join(s.units, name+".service"))
		if err != nil {
			return "", err
		}
		var command []string
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(line, "ExecStart=") {
				command = strings.Fields(strings.TrimPrefix(line, "ExecStart="))
			}
		}
		if len(command) != 4 || command[0] != flowBaft || command[1] != "run" || command[2] != "--file" {
			return "", fmt.Errorf("unexpected runtime unit command: %v", command)
		}
		cfg, err := config.LoadFile(command[3])
		if err != nil {
			return "", err
		}
		ctx, cancel := context.WithCancel(context.Background())
		p = &runtimeService{runtime: node.NewRuntime(), file: command[3], stop: cancel, done: make(chan struct{})}
		s.services[name] = p
		go func(p *runtimeService) {
			defer close(p.done)
			_ = p.runtime.Run(ctx, cfg)
		}(p)
	default:
		return "", fmt.Errorf("unsupported runtime service action %s", args[0])
	}
	return "", nil
}

func sharedOperationalAPI(t *testing.T, srv *httptest.Server, method, path string, input, output any) {
	t.Helper()
	var body bytes.Buffer
	if input != nil {
		if err := json.NewEncoder(&body).Encode(input); err != nil {
			t.Fatal(err)
		}
	}
	r, err := http.NewRequest(method, srv.URL+path, &body)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Authorization", "Bearer admin")
	r.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s %s: %d %s", method, path, resp.StatusCode, raw)
	}
	if output != nil {
		if err := json.NewDecoder(resp.Body).Decode(output); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBCCSignedSharedSameIROperationalIngressLifecycle(t *testing.T) {
	runSharedOperationalIngress(t, false)
}

func TestBCCSignedSharedSameIROperationalIngressPartialAddFailure(t *testing.T) {
	runSharedOperationalIngress(t, true)
}

// jointNFTAddFailure delegates every mutation to real nft. Its only injected
// fault is a lost response after the kernel accepted one armed tenant add.
type jointNFTAddFailure struct {
	ingressownership.NFTCommandRunner
	armed     bool
	triggered bool
}

func (r *jointNFTAddFailure) Run(ctx context.Context, args ...string) error {
	if err := r.NFTCommandRunner.Run(ctx, args...); err != nil {
		return err
	}
	if r.armed && len(args) > 0 && args[0] == "add" {
		r.armed, r.triggered = false, true
		return fmt.Errorf("LAB lost response after real kernel tenant nft add")
	}
	return nil
}

func runSharedOperationalIngress(t *testing.T, failTenantApply bool) {
	t.Helper()
	oplab.Require(t)
	// The filtered scratch LAB supplies real, separately built product binaries.
	// The ordinary TestMain continues building them for normal developer runs.
	if flowBaft == "" {
		flowBaft, flowPair = os.Getenv("BAFT_BIN"), os.Getenv("BAFT_PAIR_BIN")
		for _, binary := range []string{flowBaft, flowPair} {
			if info, err := os.Stat(binary); err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
				t.Fatalf("real flow binary unavailable: %q: %v", binary, err)
			}
		}
	}
	ctx, stop := context.WithCancel(context.Background())
	t.Cleanup(stop)
	sh := func(args ...string) []byte {
		t.Helper()
		raw, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v: %s", args, err, raw)
		}
		return raw
	}
	sh(oplab.IPBinary(), "link", "set", "lo", "up")
	if have := sh(oplab.IPBinary(), "-4", "addr", "show", "dev", "lo"); !strings.Contains(string(have), "10.77.0.1/32") {
		sh(oplab.IPBinary(), "addr", "add", "10.77.0.1/32", "dev", "lo")
	}
	sh(oplab.NFT, "add", "table", "inet", "baft")
	t.Cleanup(func() { _ = exec.Command(oplab.NFT, "delete", "table", "inet", "baft").Run() })
	sh(oplab.NFT, "add", "chain", "inet", "baft", "ingress", "{ type filter hook input priority -100; policy accept; }")
	handle, err := ingressownership.ReadNFTLabChainHandle(sh(oplab.NFT, "-j", "list", "ruleset"))
	if err != nil {
		t.Fatal(err)
	}
	sh(oplab.NFT, "add", "table", "inet", "joint_keep")
	t.Cleanup(func() { _ = exec.Command(oplab.NFT, "delete", "table", "inet", "joint_keep").Run() })
	sh(oplab.NFT, "add", "chain", "inet", "joint_keep", "keep")
	sh(oplab.NFT, "add", "rule", "inet", "joint_keep", "keep", "tcp", "dport", "2053", "accept")
	keep := sh(oplab.NFT, "-j", "list", "table", "inet", "joint_keep")
	if _, err := ingressauthority.ProvisionInstalledManagedNodePolicy(ctx, "ir-1"); err != nil {
		t.Fatal(err)
	}
	runner := &jointNFTAddFailure{NFTCommandRunner: ingressownership.NFTCommandRunner{Binary: oplab.NFT}}
	backend := &ingressownership.NFTLabBackend{Runner: runner, ExpectedChainHandle: handle}
	var operational *IngressOperational
	var authority *ingressauthority.ReservationStore
	var journal *ingressownership.DurableJournal
	var authorityDir, journalDir string
	var ownerConn net.Conn
	var ownerRule ingressownership.FirewallObservation
	var ownerReceipt *ingressauthority.ReceiptV1
	var ownerProbeAt time.Time
	opFor := func(f *flow, tn bcc.Tunnel) bcc.PublicIngressOperation {
		t.Helper()
		op, ok := f.store.GetPublicIngressOperation(tn.ID)
		if !ok || op.Command == nil || op.Dispatch == nil {
			t.Fatalf("no BCC-staged immutable ingress command for %s: %+v", tn.ID, op)
		}
		if op.Command.Binding.Port != uint16(tn.ClientPort) || op.Command.Binding.ConfigPort != uint16(tn.ClientPort) || op.Command.Binding.LocalTarget != tn.RouteListen || op.Command.Binding.PlanSHA256 != tn.PlanHash {
			t.Fatalf("ingress command lost reviewed tunnel binding: %+v tunnel=%+v", op.Command, tn)
		}
		return op
	}
	verified := func(f *flow, srv *httptest.Server, tn bcc.Tunnel) ingressownership.FirewallObservation {
		t.Helper()
		op := opFor(f, tn)
		if op.Dispatch.Phase == "held" {
			if err := f.store.EnqueueOperationalIngress(tn.ID); err != nil {
				t.Fatalf("capable enrolled Agent enqueue: %v", err)
			}
		}
		if _, err := f.ir.agent.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}
		op = opFor(f, tn)
		if op.Dispatch.Phase != "queued" || op.Receipt == nil || op.Receipt.Phase != "applied" || !op.Receipt.OwnershipVerified || op.Receipt.JobID != op.Command.JobID || op.Receipt.Binding != op.Command.Binding || len(op.Receipt.FirewallEvidence) != 1 {
			t.Fatalf("missing exact authenticated applied receipt: %+v", op)
		}
		plan, err := ingressownership.PlanIngressRule(op.Command.Binding.Port, op.Command.Binding.Proof)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := backend.Inspect(ctx, plan)
		fw := op.Receipt.FirewallEvidence[0]
		if err != nil || fw.ChainHandle != handle || actual.Handle != fw.RuleHandle || actual.Tag != fw.OwnerTag {
			t.Fatalf("receipt does not match real nft owner: %+v receipt=%+v err=%v", actual, fw, err)
		}
		var v bcc.IngressReadinessView
		sharedOperationalAPI(t, srv, "GET", "/api/ingress/readiness?tunnel_id="+tn.ID, nil, &v)
		if !v.Ready || !v.Verified || v.Card == nil || v.Card.TunnelID != tn.ID || v.Card.IRNode != tn.IRNode || v.Card.PlanSHA256 != tn.PlanHash || int(v.Card.ConfigPort) != tn.ClientPort || int(v.Card.PublicPort) != tn.ClientPort || v.Probe == nil || !v.Probe.Connected || !v.Probe.PayloadVerified {
			t.Fatalf("no verified BCC card from real probe and receipt: %+v probe=%+v", v, v.Probe)
		}
		return actual
	}
	failedTenant := func(f *flow, srv *httptest.Server, tenant bcc.Tunnel) {
		t.Helper()
		op := opFor(f, tenant)
		if !runner.triggered || op.Receipt == nil || op.Receipt.Phase != "failed" || op.Receipt.RollbackStatus != "reconciliation_required" || op.Receipt.OwnershipVerified || op.Receipt.JobID != op.Command.JobID || op.Receipt.Binding != op.Command.Binding || op.RetireCommand != nil || op.RetireReceipt != nil {
			t.Fatalf("partial add lacks truthful authenticated failed receipt: %+v", op)
		}
		var jobs []bcc.Job
		sharedOperationalAPI(t, srv, "GET", "/api/jobs", nil, &jobs)
		acknowledged := false
		for _, job := range jobs {
			if job.ID == op.Command.JobID && job.NodeID == tenant.IRNode && job.Type == agentjob.ActionPublicIngressApply && job.Status == "failed" {
				acknowledged = true
			}
		}
		if !acknowledged {
			t.Fatal("BCC did not persist the normal Agent's failed HTTPS acknowledgement")
		}
		plan, err := ingressownership.PlanIngressRule(op.Command.Binding.Port, op.Command.Binding.Proof)
		if err != nil {
			t.Fatal(err)
		}
		_, present, err := backend.ObserveOwner(ctx, plan)
		if err != nil || present {
			t.Fatalf("partial add left orphan nft owner: present=%t err=%v", present, err)
		}
		for _, endpoint := range []struct{ network, host string }{{"tcp4", "0.0.0.0"}, {"tcp6", "::"}} {
			ln, err := net.Listen(endpoint.network, net.JoinHostPort(endpoint.host, fmt.Sprint(tenant.ClientPort)))
			if err != nil {
				t.Fatalf("partial add left numeric public port bound: %s: %v", endpoint.network, err)
			}
			ln.Close()
		}
		record, found, err := journal.Current(op.Command.Binding.Proof)
		if err != nil || !found || record.Phase != ingressownership.PhaseRetired {
			t.Fatalf("partial add rollback lacks durable retired journal: %+v found=%t err=%v", record, found, err)
		}
		leftovers, err := authority.InspectFencedLeftovers()
		if err != nil {
			t.Fatal(err)
		}
		fenced := false
		for _, leftover := range leftovers {
			if leftover.Binding == op.Command.Binding && leftover.State == ingressauthority.StateNeedsReconciliation {
				fenced = true
			}
		}
		if !fenced {
			t.Fatalf("partial add lost NEEDS_RECONCILIATION fence: %+v", leftovers)
		}
		var v bcc.IngressReadinessView
		sharedOperationalAPI(t, srv, "GET", "/api/ingress/readiness?tunnel_id="+tenant.ID, nil, &v)
		if v.Ready || v.Verified || v.Card != nil {
			t.Fatalf("partial add produced deployable tenant card: %+v", v)
		}
	}
	runBCCSignedSharedSameIRRealFlows(t, sharedRealOptions{
		ctx: ctx,
		services: func(n *flowNode) sharedServices {
			return &runtimeServices{t: t, units: n.tn.UnitDir, services: map[string]*runtimeService{}, enabled: map[string]bool{}}
		},
		configure: func(f *flow, _ *httptest.Server) {
			if _, err := f.store.UpsertNode(bcc.Node{ID: f.ir.id, Address: "10.77.0.1:22", Role: "worker"}, f.ir.token); err != nil {
				t.Fatal(err)
			}
			private := func(name string) string {
				p := filepath.Join(f.dir, "operational", name)
				if err := os.MkdirAll(p, 0700); err != nil {
					t.Fatal(err)
				}
				return p
			}
			var err error
			authorityDir, journalDir = private("authority"), private("journal")
			authority, err = ingressauthority.OpenReservationStore(authorityDir)
			if err != nil {
				t.Fatal(err)
			}
			journal, err = ingressownership.OpenDurableJournal(journalDir)
			if err != nil {
				t.Fatal(err)
			}
			executor, err := ingressownership.NewOperationalExecutor(ingressownership.NewRuntime(), backend, journal)
			if err != nil {
				t.Fatal(err)
			}
			operational = &IngressOperational{Store: authority, Executor: executor}
			f.ir.agent.cfg.IngressOperational = operational
		},
		request: func(req *bcc.TunnelRequest, port func() int) { req.ClientPort = port() },
		observe: func(event string, f *flow, srv *httptest.Server, owner, tenant bcc.Tunnel) {
			t.Logf("joint checkpoint: %s; supervision=node.Runtime goroutines; pairing=actual binaries", event)
			if event == "owner" {
				ownerRule = verified(f, srv, owner)
				ownerReceipt = opFor(f, owner).Receipt
				ownerProbeAt = time.Now()
				ownerConn = readyFlow(t, fmt.Sprintf("10.77.0.1:%d", owner.ClientPort), "owner")
				t.Cleanup(func() { ownerConn.Close() })
				if failTenantApply {
					// Hold the tenant ingress through transport activation so its
					// normal signed Agent result can be inspected at the fault.
					f.ir.agent.cfg.IngressOperational = nil
				}
				return
			}
			if err := realRoundTrip(ownerConn, "owner"); err != nil {
				t.Fatalf("%s interrupted persistent public owner flow: %v", event, err)
			}
			if failTenantApply && event == "join" {
				runner.armed = true
				f.ir.agent.cfg.IngressOperational = operational
				results, err := f.ir.agent.RunOnce(ctx)
				if err != nil {
					t.Fatal(err)
				}
				op := opFor(f, tenant)
				if len(results) == 0 && op.Receipt == nil {
					// The capability poll may stage the held retry for the next
					// pull. Enqueue only through BCC's ordinary capability gate.
					if err := f.store.EnqueueOperationalIngress(tenant.ID); err != nil {
						t.Fatal(err)
					}
					results, err = f.ir.agent.RunOnce(ctx)
					if err != nil {
						t.Fatal(err)
					}
					op = opFor(f, tenant)
				}
				if len(results) != 1 || results[0].JobID != op.Command.JobID || results[0].Action != agentjob.ActionPublicIngressApply || results[0].Status != "failed" {
					t.Fatalf("partial real nft add was not a normal failed Agent result: %+v", results)
				}
				failedTenant(f, srv, tenant)
				// The supported BCC contract cannot turn failed evidence into a
				// typed Retire. Preserve this refusal and the authority fence.
				if err := f.store.EnqueueOperationalIngressRetire(tenant.ID); err == nil || err.Error() != "retire requires an exact applied ownership-verified receipt" {
					t.Fatalf("failed Apply typed Retire boundary changed: %v", err)
				} else {
					t.Logf("typed Retire unavailable for failed Apply; authority stays fenced: %v", err)
				}
			}
			if !failTenantApply && (event == "join" || event == "restart") {
				verified(f, srv, tenant)
				c := readyFlow(t, fmt.Sprintf("10.77.0.1:%d", tenant.ClientPort), "tenant")
				c.Close()
			}
			if failTenantApply {
				failedTenant(f, srv, tenant)
			}
			if failTenantApply && event == "retire" {
				// Reconstruct Agent and executor components from durable stores,
				// then run the supported reconciliation operation. The healthy
				// old owner listener is deliberately still alive: this is not an
				// Agent process death/restart or a claim that its sockets survive.
				before := opFor(f, tenant)
				leftovers, err := authority.InspectFencedLeftovers()
				if err != nil {
					t.Fatal(err)
				}
				var oldOwner ingressauthority.FencedLeftoverV1
				for _, leftover := range leftovers {
					if leftover.Binding == ownerReceipt.Binding {
						oldOwner = leftover
					}
				}
				if oldOwner.Binding != ownerReceipt.Binding {
					t.Fatal("healthy owner's durable reservation disappeared before reconciliation")
				}
				authority, err = ingressauthority.OpenReservationStore(authorityDir)
				if err != nil {
					t.Fatal(err)
				}
				journal, err = ingressownership.OpenDurableJournal(journalDir)
				if err != nil {
					t.Fatal(err)
				}
				executor, err := ingressownership.NewOperationalExecutor(ingressownership.NewRuntime(), backend, journal)
				if err != nil {
					t.Fatal(err)
				}
				cfg := f.ir.agent.cfg
				cfg.IngressOperational = &IngressOperational{Store: authority, Executor: executor}
				reopened, err := New(cfg)
				if err != nil {
					t.Fatal(err)
				}
				if _, spent := reopened.seen[before.Command.JobID]; !spent {
					t.Fatal("reconstructed Agent forgot the failed signed job's durable replay claim")
				}
				results, reconcileErr := reopened.ReconcileIngressAfterRestart(ctx, RestartReconcileAuto)
				var tenantReleased, ownerFenced bool
				for _, result := range results {
					switch result.Port {
					case before.Command.Binding.Port:
						tenantReleased = result.JobID == before.Command.JobID && result.Direction == ingressauthority.RestartApply && result.Outcome == ingressownership.OutcomeAlreadyRetired && result.Report.JournalPhase == ingressownership.PhaseRetired && !result.Report.RulePresent && strings.Contains(result.Reason, "reservation released") && strings.HasPrefix(result.Receipt, "not-posted:")
					case ownerReceipt.Binding.Port:
						ownerFenced = result.Outcome == ingressownership.OutcomeFenced
					}
				}
				if reconcileErr == nil || !tenantReleased || !ownerFenced {
					t.Fatalf("reconciliation must release only the empty retired tenant and fence the live owner: results=%+v err=%v", results, reconcileErr)
				}
				leftovers, err = authority.InspectFencedLeftovers()
				if err != nil {
					t.Fatal(err)
				}
				ownerUnchanged := false
				for _, leftover := range leftovers {
					if leftover.Binding == before.Command.Binding {
						t.Fatal("verified local recovery retained the tenant's pending claim")
					}
					if leftover == oldOwner {
						ownerUnchanged = true
					}
				}
				if !ownerUnchanged {
					t.Fatal("reconciliation changed the healthy owner's durable reservation")
				}
				raw, err := os.ReadFile(filepath.Join(authorityDir, "released.json"))
				if err != nil {
					t.Fatal(err)
				}
				var tombstones map[string]ingressauthority.ReleasedV1
				if err := json.Unmarshal(raw, &tombstones); err != nil {
					t.Fatal(err)
				}
				binding := before.Command.Binding
				tombstone := tombstones[binding.NodeID+"\x00tcp\x00"+fmt.Sprint(binding.Port)]
				if tombstone.NodeID != binding.NodeID || tombstone.Port != binding.Port || tombstone.TunnelID != binding.Spec.TunnelID || tombstone.InstanceID != binding.Spec.InstanceID || tombstone.Generation != binding.Spec.Generation || tombstone.OperationID != binding.OperationID || tombstone.LeaseID != binding.LeaseID || tombstone.PlanSHA256 != binding.PlanSHA256 || !reflect.DeepEqual(tombstone.SpentJobs, []string{before.Command.JobID}) {
					t.Fatalf("local recovery lacks the exact durable anti-replay tombstone: %+v", tombstone)
				}
				if err := authority.ReservePending(binding); !errors.Is(err, ingressauthority.ErrReserved) {
					t.Fatalf("released generation allowed old binding replay: %v", err)
				}
				if !reflect.DeepEqual(before, opFor(f, tenant)) {
					t.Fatal("local recovery fabricated or changed BCC failed-operation evidence")
				}
				var failed bcc.IngressReadinessView
				sharedOperationalAPI(t, srv, "GET", "/api/ingress/readiness?tunnel_id="+tenant.ID, nil, &failed)
				if failed.Ready || failed.Verified || failed.Card != nil {
					t.Fatalf("local recovery fabricated a deployable tenant card: %+v", failed)
				}
				f.ir.agent = reopened
				if err := realRoundTrip(ownerConn, "owner"); err != nil {
					t.Fatalf("local recovery interrupted the healthy owner's held public flow: %v", err)
				}
				t.Logf("durable component reconstruction reconciled tenant locally with anti-replay tombstone; live owner stayed fenced; BCC failed receipt and reuse boundary unchanged")
			}
			if !failTenantApply && event == "retire" {
				op := opFor(f, tenant)
				if op.RetireCommand == nil || op.RetireReceipt == nil || op.RetireCommand.JobID == op.Command.JobID || op.RetireCommand.Binding != op.Command.Binding || op.RetireReceipt.SchemaVersion != 1 || op.RetireReceipt.JobID != op.RetireCommand.JobID || op.RetireReceipt.Action != op.RetireCommand.Action || op.RetireReceipt.Binding != op.Command.Binding || op.RetireReceipt.Phase != "retired" || op.RetireReceipt.OwnershipVerified || len(op.RetireReceipt.FirewallEvidence) != 0 {
					t.Fatalf("tenant lacks exact signed retirement receipt: %+v", op)
				}
				plan, err := ingressownership.PlanIngressRule(op.Command.Binding.Port, op.Command.Binding.Proof)
				if err != nil {
					t.Fatal(err)
				}
				_, present, err := backend.ObserveOwner(ctx, plan)
				if err != nil || present {
					t.Fatalf("tenant nft rule survived retirement: present=%t err=%v", present, err)
				}
				c, err := net.DialTimeout("tcp", fmt.Sprintf("10.77.0.1:%d", tenant.ClientPort), time.Second)
				if err == nil {
					c.Close()
					t.Fatal("tenant public listener survived retirement")
				}
				var retired bcc.IngressReadinessView
				sharedOperationalAPI(t, srv, "GET", "/api/ingress/readiness?tunnel_id="+tenant.ID, nil, &retired)
				if retired.Ready || retired.Verified || retired.Card != nil {
					t.Fatalf("retired tenant still has deployable card: %+v", retired)
				}
			}
			if event == "retire" {
				// Force the normal API's next owner probe beyond its 10s cache TTL.
				for time.Since(ownerProbeAt) < 11*time.Second {
					time.Sleep(50 * time.Millisecond)
				}
			}
			if current := verified(f, srv, owner); current != ownerRule || !reflect.DeepEqual(opFor(f, owner).Receipt, ownerReceipt) {
				t.Fatalf("%s changed owner rule handle or authenticated receipt", event)
			}
			ownerProbeAt = time.Now()
			if !bytes.Equal(keep, sh(oplab.NFT, "-j", "list", "table", "inet", "joint_keep")) {
				t.Fatalf("%s changed unrelated TCP/2053 firewall rule", event)
			}
		},
	})
}
