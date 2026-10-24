//go:build linux

package ingressauthority_test

// Release gate operational-ingress-v1: the held lease is minted ONLY from a
// signed, enrolled job, under an OS-held guard, a pinned installed policy and a
// recorded intent, and dies with the guard or on uncertainty.

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agent"
	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"github.com/zarkmakerburg/baft/internal/release"
	"github.com/zarkmakerburg/baft/tests/e2e/operational/oplab"
)

const gateNode = "ir-opv1"

func gateBinding(t *testing.T, port int) ingressauthority.BindingV1 {
	t.Helper()
	spec := ingressmanifest.Spec{TunnelID: "tun-held-lease", InstanceID: "main-de", Generation: 1, Config: ingressmanifest.Config{
		Listen: "0.0.0.0:" + strconv.Itoa(port), Target: "127.0.0.1:1443", MaxClients: 128, DialTimeoutMS: 3000}}
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	return ingressauthority.BindingV1{NodeID: gateNode, OperationID: "op-held-1", LeaseID: "lease-held-1", Port: uint16(port), ConfigPort: uint16(port),
		ListenEndpoint: spec.Config.Listen, LocalTarget: spec.Config.Target, Transport: "dedicated", PlanSHA256: strings.Repeat("a1", 32),
		ListenerID: "listener-1", FirewallID: "firewall-1", Spec: spec, Proof: proof}
}

func gateJob(c ingressauthority.CommandV1) agentjob.Job {
	b := c.Binding
	p := map[string]string{
		"tunnel_id": b.Spec.TunnelID, "instance_id": b.Spec.InstanceID, "public_port": strconv.Itoa(int(b.Port)), "local_target": b.LocalTarget,
		"generation": strconv.FormatUint(b.Spec.Generation, 10), "manifest_digest": b.Proof.ManifestSHA256, "listener_id": b.ListenerID,
		"firewall_id": b.FirewallID, "ingress_protocol": ingressauthority.ProtocolV1, "operation_id": b.OperationID, "lease_id": b.LeaseID,
		"plan_sha256": b.PlanSHA256, "config_sha256": b.Proof.ConfigSHA256, "listen_endpoint": b.ListenEndpoint, "transport": b.Transport,
	}
	return agentjob.Job{SchemaVersion: agentjob.SchemaVersion, JobID: c.JobID, NodeID: b.NodeID, Action: c.Action, Params: p, IssuedAt: c.IssuedAt, ExpiresAt: c.ExpiresAt}
}

func gateCommand(b ingressauthority.BindingV1, id, action string) ingressauthority.CommandV1 {
	now := time.Now().UTC().Truncate(time.Second)
	return ingressauthority.CommandV1{IngressProtocol: ingressauthority.ProtocolV1, JobID: id, Action: action, IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Binding: b}
}

func TestOperationalIngressV1Gate_HeldLeaseMintedOnlyFromSignedEnrolledJob(t *testing.T) {
	oplab.Require(t)
	ctx := context.Background()
	if _, err := ingressauthority.ProvisionInstalledManagedNodePolicy(ctx, gateNode); err != nil {
		t.Fatal(err)
	}
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	enrolled, err := agent.New(agent.Config{BCCURL: "https://bcc.invalid", NodeID: gateNode, Token: "tok", BCCJobKey: pub, StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	sign := func(k ed25519.PrivateKey, j agentjob.Job) release.Envelope {
		t.Helper()
		env, err := agentjob.Sign(k, j)
		if err != nil {
			t.Fatal(err)
		}
		return env
	}
	dir := filepath.Join(t.TempDir(), "authority")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := ingressauthority.OpenReservationStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	b := gateBinding(t, 54830)
	apply := gateCommand(b, "held-apply-1", agentjob.ActionPublicIngressApply)
	env := sign(key, gateJob(apply))

	// An unknown owner has no guard at all.
	if g, err := store.HoldPending(b); err == nil {
		_ = g.Close()
		t.Fatal("a guard was granted for a binding that was never reserved")
	}
	if err := store.ReservePending(b); err != nil {
		t.Fatal(err)
	}
	guard, err := store.HoldPending(b)
	if err != nil {
		t.Fatal(err)
	}
	// While held, nobody else can hold the same claim.
	if g2, err := store.HoldPending(b); err == nil {
		_ = g2.Close()
		t.Fatal("two holders for one claim")
	}
	mint := func(a *agent.Agent, e release.Envelope, c ingressauthority.CommandV1, g *ingressauthority.PendingGuard, why string) {
		t.Helper()
		if l, err := a.MintIngressLease(ctx, e, c, g); err == nil || l != nil {
			t.Fatalf("lease minted: %s", why)
		}
	}
	// No policy pin, no recorded intent, no durable APPLYING state yet.
	mint(enrolled, env, apply, guard, "without pinned installed policy")
	if _, err := guard.BindInstalledPolicy(ctx); err != nil {
		t.Fatal(err)
	}
	mint(enrolled, env, apply, guard, "without a recorded command intent")
	if err := guard.RecordCommandIntent(ctx, apply); err != nil {
		t.Fatal(err)
	}
	mint(enrolled, env, apply, guard, "before the durable APPLYING mark")
	if err := guard.MarkApplying(ctx, b); err != nil {
		t.Fatal(err)
	}

	// The job itself: unsigned-by-enrollment, mis-addressed, drifted, expired.
	_, otherKey, _ := ed25519.GenerateKey(rand.Reader)
	mint(enrolled, sign(otherKey, gateJob(apply)), apply, guard, "from a job signed by a key that is not the enrolled BCC")
	wrong := gateJob(apply)
	wrong.NodeID = "ir-other"
	mint(enrolled, sign(key, wrong), apply, guard, "from a job for another node")
	drift := apply
	drift.Binding.PlanSHA256 = "b" + apply.Binding.PlanSHA256[1:]
	mint(enrolled, env, drift, guard, "for a command that differs from the signed job")
	other := apply
	other.JobID = "held-apply-other"
	mint(enrolled, env, other, guard, "for a command whose job id differs from the signed job")
	expired := apply
	expired.IssuedAt, expired.ExpiresAt = apply.IssuedAt.Add(-time.Hour), apply.IssuedAt.Add(-30*time.Minute)
	mint(enrolled, sign(key, gateJob(expired)), expired, guard, "from an expired job")
	mint(enrolled, env, apply, nil, "with no guard")
	forgedRetire := gateCommand(b, "held-retire-early", agentjob.ActionPublicIngressRetire)
	mint(enrolled, sign(key, gateJob(forgedRetire)), forgedRetire, guard, "for Retire while the state is a clean APPLYING")
	// A different enrolled node's Agent cannot use this node's job.
	foreign, err := agent.New(agent.Config{BCCURL: "https://bcc.invalid", NodeID: "ir-other", Token: "tok", BCCJobKey: pub, StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	mint(foreign, env, apply, guard, "by an Agent enrolled as another node")

	// The one valid path.
	lease, err := enrolled.MintIngressLease(ctx, env, apply, guard)
	if err != nil || lease == nil {
		t.Fatalf("held, signed, enrolled, pinned Apply refused: %v", err)
	}
	if err := lease.CheckLockedIngress(ctx, apply); err != nil {
		t.Fatalf("minted lease not live: %v", err)
	}
	for name, c := range map[string]ingressauthority.CommandV1{"drifted binding": drift, "other job": other, "retire": forgedRetire} {
		if err := lease.CheckLockedIngress(ctx, c); err == nil {
			t.Fatalf("lease authorised a %s", name)
		}
	}
	if err := (&ingressauthority.Lease{}).CheckLockedIngress(ctx, apply); err == nil {
		t.Fatal("a zero Lease authorised a command")
	}
	if _, err := json.Marshal(lease); err == nil {
		t.Fatal("a lease serialises (it could be forged from JSON)")
	}
	if err := json.Unmarshal([]byte(`{}`), &ingressauthority.Lease{}); err == nil {
		t.Fatal("a lease deserialises")
	}
	// Single use, also across a restart of the holder.
	if l, err := enrolled.MintIngressLease(ctx, env, apply, guard); !errors.Is(err, ingressauthority.ErrLeaseReplay) || l != nil {
		t.Fatalf("second mint for the same signed job: %v", err)
	}

	// Uncertainty revokes the Apply lease; a NEW signed Retire may then mint.
	if err := guard.RecordReconciliationNeeded(ctx, apply); err != nil {
		t.Fatal(err)
	}
	if err := lease.CheckLockedIngress(ctx, apply); err == nil {
		t.Fatal("the Apply lease survived NEEDS_RECONCILIATION")
	}
	retire := gateCommand(b, "held-retire-1", agentjob.ActionPublicIngressRetire)
	renv := sign(key, gateJob(retire))
	mint(enrolled, renv, retire, guard, "for Retire without its own recorded intent")
	if err := guard.RecordCommandIntent(ctx, retire); err != nil {
		t.Fatal(err)
	}
	rl, err := enrolled.MintIngressLease(ctx, renv, retire, guard)
	if err != nil || rl == nil {
		t.Fatalf("a new signed Retire from the uncertain state was refused: %v", err)
	}
	if err := rl.CheckLockedIngress(ctx, retire); err != nil {
		t.Fatal(err)
	}
	if err := rl.CheckLockedIngress(ctx, apply); err == nil {
		t.Fatal("a Retire lease authorised Apply")
	}
	// The lease dies with the guard...
	if err := guard.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rl.CheckLockedIngress(ctx, retire); err == nil {
		t.Fatal("the lease outlived its guard")
	}
	mint(enrolled, renv, retire, guard, "from a closed guard")
	// ...and no restarted holder can re-mint anything already consumed.
	reopened, err := ingressauthority.OpenReservationStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	g2, err := reopened.HoldPending(b)
	if err != nil {
		t.Fatal(err)
	}
	defer g2.Close()
	if _, err := g2.BindInstalledPolicy(ctx); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		env release.Envelope
		cmd ingressauthority.CommandV1
	}{"apply": {env, apply}, "retire": {renv, retire}} {
		if l, err := enrolled.MintIngressLease(ctx, c.env, c.cmd, g2); err == nil || l != nil {
			t.Fatalf("restarted holder re-minted the consumed %s", name)
		}
	}
}
