//go:build linux

// Command recoverylab is the SOLE process of one lab incarnation of the
// operational-ingress Agent for the restart-recovery tests. The product's
// inventory only accepts a private network namespace with exactly one process,
// so the orchestrating test cannot live in that namespace: it starts this
// program through `ip netns exec`, waits for a fault marker, SIGKILLs it, and
// starts the next incarnation over the same durable directories.
//
// It uses the real components (ReservationStore, DurableJournal, Runtime,
// NFT backend, OperationalExecutor, Agent.MintIngressLease and
// Agent.ReconcileIngressAfterRestart); only the BCC transport is absent.
package main

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/zarkmakerburg/baft/internal/agent"
	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"github.com/zarkmakerburg/baft/internal/ingressownership"
	"github.com/zarkmakerburg/baft/internal/release"
)

const node = "ir-rec"

type labCtx struct {
	dir    string
	port   int
	fault  string
	handle uint64
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "recoverylab: "+format+"\n", a...)
	os.Exit(3)
}

func result(v any) {
	raw, _ := json.Marshal(v)
	fmt.Printf("RESULT %s\n", raw)
}

func priv(dir string) ed25519.PrivateKey {
	p := filepath.Join(dir, "jobkey")
	seed, err := os.ReadFile(p)
	if err != nil || len(seed) != ed25519.SeedSize {
		seed = make([]byte, ed25519.SeedSize)
		for i := range seed {
			seed[i] = byte(i*7 + 3)
		}
		if err := os.WriteFile(p, seed, 0o600); err != nil {
			die("jobkey: %v", err)
		}
	}
	return ed25519.NewKeyFromSeed(seed)
}

func binding(port int) ingressauthority.BindingV1 {
	spec := ingressmanifest.Spec{TunnelID: "rec-t1", InstanceID: "rec1", Generation: 1, Config: ingressmanifest.Config{
		Listen: "0.0.0.0:" + strconv.Itoa(port), Target: "127.0.0.1:1443", MaxClients: 128, DialTimeoutMS: 3000}}
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		die("preview: %v", err)
	}
	b := ingressauthority.BindingV1{NodeID: node, OperationID: "op-rec-1", LeaseID: "lease-rec-1", Port: uint16(port), ConfigPort: uint16(port),
		ListenEndpoint: spec.Config.Listen, LocalTarget: spec.Config.Target, Transport: "dedicated",
		PlanSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ListenerID: "listener-rec-1", FirewallID: "firewall-rec-1",
		Spec: spec, Proof: proof}
	if err := b.Validate(); err != nil {
		die("binding: %v", err)
	}
	return b
}

func jobParams(b ingressauthority.BindingV1) map[string]string {
	p := map[string]string{
		"tunnel_id": b.Spec.TunnelID, "public_port": strconv.Itoa(int(b.Port)), "local_target": b.LocalTarget,
		"generation": strconv.FormatUint(b.Spec.Generation, 10), "manifest_digest": b.Proof.ManifestSHA256,
		"listener_id": b.ListenerID, "firewall_id": b.FirewallID, "ingress_protocol": ingressauthority.ProtocolV1,
		"operation_id": b.OperationID, "lease_id": b.LeaseID, "plan_sha256": b.PlanSHA256,
		"config_sha256": b.Proof.ConfigSHA256, "listen_endpoint": b.ListenEndpoint, "transport": b.Transport,
		"instance_id": b.Spec.InstanceID,
	}
	return p
}

// command returns the stored command for a job (created on first use so every
// incarnation signs byte-identical windows).
func (l *labCtx) command(action, jobID string) ingressauthority.CommandV1 {
	p := filepath.Join(l.dir, "cmd-"+jobID+".json")
	if raw, err := os.ReadFile(p); err == nil {
		var c ingressauthority.CommandV1
		if json.Unmarshal(raw, &c) == nil {
			return c
		}
	}
	now := time.Now().UTC().Truncate(time.Second)
	c := ingressauthority.CommandV1{IngressProtocol: ingressauthority.ProtocolV1, JobID: jobID, Action: action,
		IssuedAt: now.Add(-2 * time.Second), ExpiresAt: now.Add(10 * time.Minute), Binding: binding(l.port)}
	raw, _ := json.Marshal(c)
	if err := os.WriteFile(p, raw, 0o600); err != nil {
		die("store command: %v", err)
	}
	return c
}

func (l *labCtx) envelope(c ingressauthority.CommandV1) release.Envelope {
	j := agentjob.Job{JobID: c.JobID, NodeID: node, Action: c.Action, Params: jobParams(c.Binding), IssuedAt: c.IssuedAt, ExpiresAt: c.ExpiresAt}
	e, err := agentjob.Sign(priv(l.dir), j)
	if err != nil {
		die("sign: %v", err)
	}
	return e
}

// ---- fault gates: write a marker, then block until SIGKILL ----

func (l *labCtx) hit(point string) {
	if l.fault != point {
		return
	}
	_ = os.WriteFile(filepath.Join(l.dir, "at-"+point), []byte("x"), 0o600)
	select {}
}

type faultRunner struct {
	ingressownership.NFTCommandRunner
	l *labCtx
}

func (r *faultRunner) Run(ctx context.Context, args ...string) error {
	if len(args) > 0 {
		r.l.hit("before:" + args[0]) // before:delete / before:add
	}
	err := r.NFTCommandRunner.Run(ctx, args...)
	if err == nil && len(args) > 0 {
		r.l.hit("after:" + args[0])
	}
	return err
}

type faultJournal struct {
	ingressownership.MutationJournal
	l *labCtx
}

func (j *faultJournal) Advance(r ingressownership.MutationRecord) error {
	j.l.hit("before:" + string(r.Phase))
	if err := j.MutationJournal.Advance(r); err != nil {
		return err
	}
	j.l.hit("after:" + string(r.Phase))
	return nil
}

type rig struct {
	store *ingressauthority.ReservationStore
	exec  *ingressownership.OperationalExecutor
	agent *agent.Agent
	dj    *ingressownership.DurableJournal
}

func private(p string) string {
	if err := os.MkdirAll(p, 0o700); err != nil {
		die("mkdir %s: %v", p, err)
	}
	_ = os.Chmod(p, 0o700)
	return p
}

func (l *labCtx) open() *rig {
	raw, err := exec.Command("/usr/sbin/nft", "-j", "list", "ruleset").Output()
	if err != nil {
		die("nft: %v", err)
	}
	h, err := ingressownership.ReadNFTLabChainHandle(raw)
	if err != nil {
		die("chain handle: %v", err)
	}
	l.handle = h
	store, err := ingressauthority.OpenReservationStore(private(filepath.Join(l.dir, "authority")))
	if err != nil {
		die("store: %v", err)
	}
	dj, err := ingressownership.OpenDurableJournal(private(filepath.Join(l.dir, "journal")))
	if err != nil {
		die("journal: %v", err)
	}
	backend := &ingressownership.NFTLabBackend{Runner: &faultRunner{NFTCommandRunner: ingressownership.NFTCommandRunner{Binary: "/usr/sbin/nft"}, l: l}, ExpectedChainHandle: h}
	x, err := ingressownership.NewOperationalExecutor(ingressownership.NewRuntime(), backend, &faultJournal{MutationJournal: dj, l: l})
	if err != nil {
		die("executor: %v", err)
	}
	a, err := agent.New(agent.Config{BCCURL: "https://127.0.0.1:9", NodeID: node, Token: "lab-token", BCCJobKey: priv(l.dir).Public().(ed25519.PublicKey),
		StateDir: private(filepath.Join(l.dir, "agent")), IngressOperational: &agent.IngressOperational{Store: store, Executor: x}})
	if err != nil {
		die("agent: %v", err)
	}
	return &rig{store: store, exec: x, agent: a, dj: dj}
}

// admit replicates the pre-effect steps of Agent.publicIngressOperational for
// one signed command and returns the held guard plus the single-use lease.
func (l *labCtx) admit(ctx context.Context, r *rig, c ingressauthority.CommandV1, markApplying, mint bool) (*ingressauthority.PendingGuard, *ingressauthority.Lease, error) {
	if c.Action == agentjob.ActionPublicIngressApply {
		if err := r.store.ReservePending(c.Binding); err != nil {
			return nil, nil, fmt.Errorf("reserve: %w", err)
		}
	}
	g, err := r.store.HoldPending(c.Binding)
	if err != nil {
		return nil, nil, fmt.Errorf("hold: %w", err)
	}
	if _, err := g.BindInstalledPolicy(ctx); err != nil {
		g.Close()
		return nil, nil, fmt.Errorf("policy: %w", err)
	}
	if err := g.RecordCommandIntent(ctx, c); err != nil {
		g.Close()
		return nil, nil, fmt.Errorf("intent: %w", err)
	}
	if markApplying {
		if err := g.MarkApplying(ctx, c.Binding); err != nil {
			g.Close()
			return nil, nil, fmt.Errorf("mark: %w", err)
		}
	}
	if !mint {
		return g, nil, nil
	}
	lease, err := r.agent.MintIngressLease(ctx, l.envelope(c), c, g)
	if err != nil {
		g.Close()
		return nil, nil, fmt.Errorf("mint: %w", err)
	}
	return g, lease, nil
}

func (l *labCtx) doApply(ctx context.Context, r *rig) error {
	c := l.command(agentjob.ActionPublicIngressApply, "apply-rec-1")
	g, lease, err := l.admit(ctx, r, c, true, true)
	if err != nil {
		return err
	}
	defer g.Close()
	_, err = r.exec.Apply(ctx, c, lease)
	return err
}

func (l *labCtx) doRetire(ctx context.Context, r *rig, jobID string) error {
	c := l.command(agentjob.ActionPublicIngressRetire, jobID)
	g, lease, err := l.admit(ctx, r, c, false, true)
	if err != nil {
		return err
	}
	defer g.Close()
	_, err = r.exec.Retire(ctx, c, lease)
	return err
}

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func main() {
	dir := flag.String("dir", "", "durable lab state directory")
	port := flag.Int("port", 0, "public ingress port")
	fault := flag.String("fault", "", "block (until SIGKILL) at this point, e.g. before:delete, after:retire_intent")
	foreign := flag.String("foreign", "", "bind a foreign listener on the port (tcp4|tcp6) before reconciling")
	mode := flag.String("mode", "auto", "reconcile mode: auto|report")
	flag.Parse()
	if *dir == "" || *port == 0 || flag.NArg() != 1 {
		die("usage: recoverylab -dir D -port P [-fault F] phase")
	}
	l := &labCtx{dir: private(*dir), port: *port, fault: *fault}
	ctx := context.Background()
	switch flag.Arg(0) {
	case "init":
		if _, err := ingressauthority.ProvisionInstalledManagedNodePolicy(ctx, node); err != nil {
			die("provision policy: %v", err)
		}
		result(map[string]any{"ok": true})
	case "plant":
		// An owner-tagged rule that no journal or lease explains.
		plan, err := ingressownership.PlanIngressRule(uint16(*port), binding(*port).Proof)
		if err != nil {
			die("plan: %v", err)
		}
		out, err := exec.Command("/usr/sbin/nft", "add", "rule", "inet", "baft", "ingress", "tcp", "dport", strconv.Itoa(*port), "accept", "comment", plan.Tag).CombinedOutput()
		result(map[string]any{"err": errStr(err), "out": string(out), "tag": plan.Tag})
	case "apply":
		r := l.open()
		result(map[string]any{"err": errStr(l.doApply(ctx, r))})
	case "apply-retire":
		r := l.open()
		if err := l.doApply(ctx, r); err != nil {
			result(map[string]any{"err": "apply: " + err.Error()})
			return
		}
		result(map[string]any{"err": errStr(l.doRetire(ctx, r, "retire-rec-1"))})
	case "intent-only":
		r := l.open()
		c := l.command(agentjob.ActionPublicIngressApply, "apply-rec-1")
		g, _, err := l.admit(ctx, r, c, true, false)
		if err == nil {
			g.Close()
		}
		result(map[string]any{"err": errStr(err)})
	case "mint-spent":
		// A spent signed job must never mint again, in any incarnation.
		r := l.open()
		jobs := map[string]string{"apply": "apply-rec-1", "retire": "retire-rec-1"}
		action := map[string]string{"apply": agentjob.ActionPublicIngressApply, "retire": agentjob.ActionPublicIngressRetire}
		which := os.Getenv("SPENT")
		c := l.command(action[which], jobs[which])
		g, lease, err := l.admit(ctx, r, c, false, true)
		if g != nil {
			g.Close()
		}
		result(map[string]any{"err": errStr(err), "minted": lease != nil})
	case "retire-new":
		// A NEW signed Retire after recovery: inert against a retired journal.
		r := l.open()
		result(map[string]any{"err": errStr(l.doRetire(ctx, r, "retire-rec-2"))})
	case "reconcile":
		r := l.open()
		var ln net.Listener
		if *foreign != "" {
			addr := "0.0.0.0:" + strconv.Itoa(*port)
			if *foreign == "tcp6" {
				addr = "[::]:" + strconv.Itoa(*port)
			}
			var err error
			if ln, err = net.Listen(*foreign, addr); err != nil {
				die("foreign listener: %v", err)
			}
			defer ln.Close()
		}
		res, err := r.agent.ReconcileIngressAfterRestart(ctx, agent.RestartReconcileMode(*mode))
		result(map[string]any{"err": errStr(err), "results": res})
	case "probe":
		r := l.open()
		b := binding(l.port)
		rec, found, err := r.dj.Current(b.Proof)
		left, lerr := r.store.InspectFencedLeftovers()
		var states []string
		for _, x := range left {
			states = append(states, string(x.State))
		}
		intents, _ := r.store.InspectCommandIntents(b)
		var jobs []string
		for _, c := range intents {
			jobs = append(jobs, c.JobID)
		}
		result(map[string]any{"phase": rec.Phase, "found": found, "handle": rec.FirewallHandle, "err": errStr(err) + errStr(lerr), "states": states, "intents": jobs})
	default:
		die("unknown phase %q", flag.Arg(0))
	}
}
