//go:build linux

package ingressauthority

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressinventory"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

func isolatedPolicyBootstrapLAB(t *testing.T) {
	t.Helper()
	if os.Getenv("BAFT_ISOLATED_POLICY_LAB") != "1" {
		t.Skip("requires disposable Linux root with explicit LAB opt-in")
	}
	sentinel, err := os.ReadFile("/baft-policy-lab")
	if os.Geteuid() != 0 || err != nil || string(sentinel) != "isolated-policy-lab" {
		t.Fatal("refusing bootstrap tests outside explicitly prepared disposable root")
	}
	for _, path := range []string{"/etc/baft", "/etc/systemd", "/usr/lib/systemd"} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("refusing pre-existing host state: %s %v", path, err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(path) })
	}
	for _, path := range []string{"/etc/systemd/system", "/usr/lib/systemd/system"} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProvisionInstalledPolicyLABPersistsSharedIdentity(t *testing.T) {
	isolatedPolicyBootstrapLAB(t)
	foreign := []byte("[Socket]\nListenStream=54619\n")
	if err := os.WriteFile("/etc/systemd/system/foreign.socket", foreign, 0600); err != nil {
		t.Fatal(err)
	}
	p, err := ProvisionInstalledManagedNodePolicy(context.Background(), "ir-policy")
	if err != nil {
		t.Fatal(err)
	}
	if p.ProfilePath != ingressinventory.InstalledPolicyPath || p.NodeID != "ir-policy" || p.ProfileInode == 0 || len(p.ProfileSHA256) != 64 || len(p.SourceRoots) != 2 {
		t.Fatalf("invalid installed identity: %+v", p)
	}
	fi, err := os.Stat(p.ProfilePath)
	if err != nil || fi.Mode().Perm() != 0600 {
		t.Fatalf("policy not owner-only: %v %v", fi, err)
	}
	st := fi.Sys().(*syscall.Stat_t)
	if st.Uid != 0 {
		t.Fatal("policy not root-owned")
	}
	parent, err := os.Stat(filepath.Dir(p.ProfilePath))
	if err != nil || parent.Mode().Perm() != 0700 {
		t.Fatal("policy directory not private")
	}
	repeat, err := ProvisionInstalledManagedNodePolicy(context.Background(), "ir-policy")
	if err != nil || !reflect.DeepEqual(p, repeat) {
		t.Fatalf("idempotent provision replaced installed policy: %+v %v", repeat, err)
	}
	loaded, err := ingressinventory.LoadInstalledManagedNodePolicy(context.Background(), "ir-policy")
	if err != nil || !reflect.DeepEqual(p, loaded) {
		t.Fatal("provision bypassed shared trusted decoder")
	}
	raw, err := os.ReadFile(p.ProfilePath)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := ingressinventory.MarshalInstalledManagedNodePolicyV1(context.Background(), p)
	if err != nil || !bytes.Equal(raw, encoded) {
		t.Fatal("installer duplicated/diverged from leaf wire schema")
	}
	after, err := os.ReadFile("/etc/systemd/system/foreign.socket")
	if err != nil || !bytes.Equal(after, foreign) {
		t.Fatal("bootstrap modified an existing service")
	}
	if _, err := ProvisionInstalledManagedNodePolicy(context.Background(), "different-node"); err == nil {
		t.Fatal("bootstrap replaced another node's installed identity")
	}
	after, err = os.ReadFile(p.ProfilePath)
	if err != nil || !bytes.Equal(raw, after) {
		t.Fatal("refused re-pair changed installed profile")
	}
	evidence, err := ingressinventory.CollectInstalledManagedTCP(context.Background(), "ir-policy", 54620)
	if !errors.Is(err, ingressinventory.ErrCoverageIncomplete) || evidence.CoverageComplete || !evidence.SupportedEnvelope {
		t.Fatalf("policy install manufactured operational coverage: %+v %v", evidence, err)
	}
}

func TestProvisionInstalledPolicyLABRefusesUnsafeExistingState(t *testing.T) {
	for _, kind := range []string{"writable-parent", "symlink-parent", "nonroot-parent", "fifo-file", "malformed-file", "namespace-drift", "writable-source"} {
		t.Run(kind, func(t *testing.T) {
			isolatedPolicyBootstrapLAB(t)
			if kind == "writable-source" {
				if err := os.Chmod("/etc/systemd/system", 0777); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.MkdirAll("/etc/baft/ingress", 0700); err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "writable-parent":
					if err := os.Chmod("/etc/baft/ingress", 0777); err != nil {
						t.Fatal(err)
					}
				case "symlink-parent":
					if err := os.Remove("/etc/baft/ingress"); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink("/etc/systemd/system", "/etc/baft/ingress"); err != nil {
						t.Fatal(err)
					}
				case "nonroot-parent":
					if err := os.Chown("/etc/baft/ingress", 501, 501); err != nil {
						t.Fatal(err)
					}
				case "fifo-file":
					if err := syscall.Mkfifo(ingressinventory.InstalledPolicyPath, 0600); err != nil {
						t.Fatal(err)
					}
				case "malformed-file":
					if err := os.WriteFile(ingressinventory.InstalledPolicyPath, []byte("{}"), 0600); err != nil {
						t.Fatal(err)
					}
				case "namespace-drift":
					if _, err := ProvisionInstalledManagedNodePolicy(context.Background(), "ir-policy"); err != nil {
						t.Fatal(err)
					}
					raw, err := os.ReadFile(ingressinventory.InstalledPolicyPath)
					if err != nil {
						t.Fatal(err)
					}
					ns, err := os.Readlink("/proc/self/ns/net")
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(ingressinventory.InstalledPolicyPath, []byte(strings.Replace(string(raw), ns, "net:[0]", 1)), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			if _, err := ProvisionInstalledManagedNodePolicy(context.Background(), "ir-policy"); err == nil {
				t.Fatal("untrusted existing installer state was adopted or overwritten")
			}
			if kind == "malformed-file" {
				raw, err := os.ReadFile(ingressinventory.InstalledPolicyPath)
				if err != nil || string(raw) != "{}" {
					t.Fatal("invalid existing policy overwritten")
				}
			}
			if kind == "symlink-parent" {
				if _, err := os.Lstat("/etc/systemd/system/managed-node.json"); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("installer followed foreign parent symlink")
				}
			}
		})
	}
}

func TestProvisionInstalledPolicyLABConcurrentNoReplace(t *testing.T) {
	isolatedPolicyBootstrapLAB(t)
	var wg sync.WaitGroup
	results := make(chan ingressinventory.InstalledManagedNodePolicyV1, 4)
	failures := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Go(func() {
			p, err := ProvisionInstalledManagedNodePolicy(context.Background(), "ir-concurrent")
			results <- p
			failures <- err
		})
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first ingressinventory.InstalledManagedNodePolicyV1
	for p := range results {
		if first.ProfileInode == 0 {
			first = p
		} else if !reflect.DeepEqual(first, p) {
			t.Fatal("concurrent bootstrap replaced a policy identity")
		}
	}
	entries, err := os.ReadDir("/etc/baft/ingress")
	if err != nil || len(entries) != 1 || entries[0].Name() != "managed-node.json" {
		t.Fatalf("incomplete bootstrap left artifacts: %+v %v", entries, err)
	}
}

func TestProvisionInstalledPolicyLABCancellationAndRootRequirement(t *testing.T) {
	isolatedPolicyBootstrapLAB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ProvisionInstalledManagedNodePolicy(ctx, "ir-policy"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled bootstrap changed state: %v", err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestProvisionInstalledPolicyNonRootHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(), "BAFT_POLICY_UID_HELPER=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 501, Gid: 501}}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("nonroot installer refusal: %v %s", err, out)
	}
	if _, err := os.Stat("/etc/baft"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("refused bootstrap created policy state")
	}
}

func TestProvisionInstalledPolicyNonRootHelper(t *testing.T) {
	if os.Getenv("BAFT_POLICY_UID_HELPER") != "1" {
		t.Skip("subprocess only")
	}
	if os.Geteuid() == 0 {
		t.Fatal("root helper did not change uid")
	}
	if _, err := ProvisionInstalledManagedNodePolicy(context.Background(), "ir-policy"); err == nil {
		t.Fatal("nonroot bootstrap accepted")
	}
}

func TestProvisionInstalledPolicyLABCompetingNodeIdentity(t *testing.T) {
	isolatedPolicyBootstrapLAB(t)
	type outcome struct {
		node string
		p    ingressinventory.InstalledManagedNodePolicyV1
		err  error
	}
	done := make(chan outcome, 2)
	for _, node := range []string{"ir-first", "ir-second"} {
		go func() {
			p, err := ProvisionInstalledManagedNodePolicy(context.Background(), node)
			done <- outcome{node, p, err}
		}()
	}
	wins := 0
	var winner outcome
	for i := 0; i < 2; i++ {
		result := <-done
		if result.err == nil {
			wins++
			winner = result
		}
	}
	if wins != 1 {
		t.Fatalf("conflicting concurrent node identities accepted: %d", wins)
	}
	loaded, err := ingressinventory.LoadInstalledManagedNodePolicy(context.Background(), winner.node)
	if err != nil || !reflect.DeepEqual(loaded, winner.p) {
		t.Fatal("winning installed identity was replaced")
	}
}

func TestInstalledPolicyBindingLABSurvivesJournalAndRefusesInodeDrift(t *testing.T) {
	isolatedPolicyBootstrapLAB(t)
	if _, err := ProvisionInstalledManagedNodePolicy(context.Background(), "ir-policy"); err != nil {
		t.Fatal(err)
	}
	b := bindingFixture(t)
	b.NodeID = "ir-policy"
	store, err := OpenReservationStore(privateReservationDir(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReservePending(b); err != nil {
		t.Fatal(err)
	}
	guard, err := store.HoldPending(b)
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.CheckInstalledPolicy(context.Background()); err == nil {
		t.Fatal("unbound policy accepted")
	}
	pinned, err := guard.BindInstalledPolicy(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if pinned.NodeID != b.NodeID || pinned.ProfileInode == 0 || pinned.ProfilePath != ingressinventory.InstalledPolicyPath {
		t.Fatal("held journal lost trusted policy identity")
	}
	if err := guard.MarkApplying(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	if err := guard.Close(); err != nil {
		t.Fatal(err)
	}
	if err := guard.CheckInstalledPolicy(context.Background()); err == nil {
		t.Fatal("closed OS guard checked policy")
	}
	restarted, err := OpenReservationStore(store.dir)
	if err != nil {
		t.Fatal(err)
	}
	state, err := restarted.InspectRecovery(b)
	if err != nil || state != StateNeedsReconciliation {
		t.Fatal("policy pin promoted incomplete effects to ready")
	}
	guard, err = restarted.HoldPending(b)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	if err := guard.CheckInstalledPolicy(context.Background()); err != nil {
		t.Fatalf("MarkApplying/restart erased durable policy pin: %v", err)
	}
	repeat, err := guard.BindInstalledPolicy(context.Background())
	if err != nil || repeat != pinned {
		t.Fatal("idempotent policy pin changed identity")
	}
	raw, err := os.ReadFile(ingressinventory.InstalledPolicyPath)
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(filepath.Dir(ingressinventory.InstalledPolicyPath), "same-bytes-new-inode")
	if err := os.WriteFile(replacement, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, ingressinventory.InstalledPolicyPath); err != nil {
		t.Fatal(err)
	}
	if err := guard.CheckInstalledPolicy(context.Background()); err == nil {
		t.Fatal("same bytes on a replaced policy inode accepted")
	}
	if _, err := guard.BindInstalledPolicy(context.Background()); err == nil {
		t.Fatal("drift silently rebound an unresolved operation")
	}
	if err := guard.Close(); err != nil {
		t.Fatal(err)
	}
	other := b
	other.LeaseID = "competing-after-drift"
	if err := restarted.ReservePending(other); err == nil {
		t.Fatal("policy drift released durable pending fence")
	}
}

func TestInstalledPolicyBindingLABRejectsMalformedDurableIdentity(t *testing.T) {
	isolatedPolicyBootstrapLAB(t)
	if _, err := ProvisionInstalledManagedNodePolicy(context.Background(), "ir-policy"); err != nil {
		t.Fatal(err)
	}
	b := bindingFixture(t)
	b.NodeID = "ir-policy"
	store, err := OpenReservationStore(privateReservationDir(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReservePending(b); err != nil {
		t.Fatal(err)
	}
	guard, err := store.HoldPending(b)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := guard.BindInstalledPolicy(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := guard.Close(); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(store.dir, lifecycleFile)
	original, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*InstalledPolicyIdentityV1){
		func(p *InstalledPolicyIdentityV1) { p.NodeID = "other-node" },
		func(p *InstalledPolicyIdentityV1) { p.ProfilePath = "/tmp/caller-policy.json" },
		func(p *InstalledPolicyIdentityV1) { p.ProfileInode = 0 },
		func(p *InstalledPolicyIdentityV1) { p.ProfileSHA256 = "unknown" },
		func(p *InstalledPolicyIdentityV1) { p.InventoryVersion++ },
		func(p *InstalledPolicyIdentityV1) { p.NetworkNamespace = "unknown" },
	} {
		var entries map[string]lifecycleEntry
		if err := json.Unmarshal(original, &entries); err != nil {
			t.Fatal(err)
		}
		entry := entries[reservationKey(b)]
		change(entry.Policy)
		raw, err := json.Marshal(entries)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, raw, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.InspectRecovery(b); err == nil {
			t.Fatal("malformed durable installed-policy identity accepted")
		}
	}
}

func TestCommandIntentLABSurvivesRestartAndFencesReplays(t *testing.T) {
	isolatedPolicyBootstrapLAB(t)
	if _, err := ProvisionInstalledManagedNodePolicy(context.Background(), "ir-policy"); err != nil {
		t.Fatal(err)
	}
	b := bindingFixture(t)
	b.NodeID = "ir-policy"
	store, err := OpenReservationStore(privateReservationDir(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReservePending(b); err != nil {
		t.Fatal(err)
	}
	g, err := store.HoldPending(b)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.BindInstalledPolicy(context.Background()); err != nil {
		t.Fatal(err)
	}
	apply := commandIntentFixture(t, b)
	if err := g.RecordCommandIntent(context.Background(), apply); err != nil {
		t.Fatal(err)
	}
	if err := g.CheckHeldCommand(context.Background(), apply); err != nil {
		t.Fatalf("exact held policy-bound command unavailable: %v", err)
	}
	wrongJob := apply
	wrongJob.JobID = "different-signed-job"
	if err := g.CheckHeldCommand(context.Background(), wrongJob); err == nil {
		t.Fatal("nonmatching signed job accessed held authority")
	}
	if err := g.RecordCommandIntent(context.Background(), apply); err != nil {
		t.Fatalf("identical command retry failed: %v", err)
	}
	changed := apply
	changed.JobID = "replacement-apply"
	if err := g.RecordCommandIntent(context.Background(), changed); err == nil {
		t.Fatal("new job silently replaced immutable Apply intent")
	}
	changed = apply
	changed.Binding.PlanSHA256 = strings.Repeat("b", 64)
	if err := g.RecordCommandIntent(context.Background(), changed); err == nil {
		t.Fatal("reviewed binding drift recorded")
	}
	retire := apply
	retire.Action = "public_ingress_retire"
	if err := g.RecordCommandIntent(context.Background(), retire); err == nil {
		t.Fatal("Retire reused Apply signed job identity")
	}
	retire.JobID = "retire-intent-2"
	// Journal order, not a monotonic wall clock, establishes request order.
	retire.IssuedAt = apply.IssuedAt.Add(-time.Second)
	if err := g.RecordCommandIntent(context.Background(), retire); err != nil {
		t.Fatal(err)
	}
	if err := g.MarkApplying(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	if err := g.RecordReconciliationNeeded(context.Background(), apply); err != nil {
		t.Fatalf("could not durably fence uncertain owned resource: %v", err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenReservationStore(store.dir)
	if err != nil {
		t.Fatal(err)
	}
	intents, err := reopened.InspectCommandIntents(b)
	if err != nil || len(intents) != 2 || intents[0].JobID != apply.JobID || intents[1].JobID != retire.JobID || intents[0].Binding != b || intents[1].Binding != b {
		t.Fatalf("restart lost exact signed-command correlation: %+v %v", intents, err)
	}
	intents[0].JobID = "caller-mutated"
	again, err := reopened.InspectCommandIntents(b)
	if err != nil || again[0].JobID != apply.JobID {
		t.Fatal("readback exposed mutable durable state")
	}
	state, err := reopened.InspectRecovery(b)
	if err != nil || state != StateNeedsReconciliation {
		t.Fatal("command intent became ready or freed a resource")
	}
	other := b
	other.LeaseID = "different-lease"
	if err := reopened.ReservePending(other); err == nil {
		t.Fatal("command restart released pending resource fence")
	}
	if err := (&LiveLease{}).CheckLockedIngress(context.Background(), apply); err == nil {
		t.Fatal("journal data minted a live lease")
	}
}

func TestCommandIntentLABRejectsCrossOperationJobAndRetireWithoutApply(t *testing.T) {
	isolatedPolicyBootstrapLAB(t)
	if _, err := ProvisionInstalledManagedNodePolicy(context.Background(), "ir-policy"); err != nil {
		t.Fatal(err)
	}
	store, err := OpenReservationStore(privateReservationDir(t))
	if err != nil {
		t.Fatal(err)
	}
	first := bindingFixture(t)
	first.NodeID = "ir-policy"
	if err := store.ReservePending(first); err != nil {
		t.Fatal(err)
	}
	g, err := store.HoldPending(first)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.BindInstalledPolicy(context.Background()); err != nil {
		t.Fatal(err)
	}
	apply := commandIntentFixture(t, first)
	if err := g.RecordCommandIntent(context.Background(), apply); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	second := first
	second.Port++
	second.ConfigPort = second.Port
	second.ListenEndpoint = "0.0.0.0:" + strconv.Itoa(int(second.Port))
	second.Spec.Config.Listen = second.ListenEndpoint
	second.OperationID = "op-second"
	second.LeaseID = "lease-second"
	second.Spec.TunnelID = "tunnel-second"
	second.Proof, err = ingressmanifest.Preview(second.Spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReservePending(second); err != nil {
		t.Fatal(err)
	}
	g, err = store.HoldPending(second)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	if _, err := g.BindInstalledPolicy(context.Background()); err != nil {
		t.Fatal(err)
	}
	candidate := apply
	candidate.Binding = second
	candidate.Action = "public_ingress_retire"
	candidate.JobID = "retire-without-apply"
	if err := g.RecordCommandIntent(context.Background(), candidate); err == nil {
		t.Fatal("Retire intent before matching Apply accepted")
	}
	candidate.Action = "public_ingress_apply"
	candidate.JobID = apply.JobID
	if err := g.RecordCommandIntent(context.Background(), candidate); err == nil {
		t.Fatal("signed job ID shared by independent resource intents")
	}
	candidate.JobID = "second-apply"
	if err := g.RecordCommandIntent(context.Background(), candidate); err != nil {
		t.Fatal(err)
	}
}

func TestCommandIntentLABRejectsMalformedDurableRequest(t *testing.T) {
	isolatedPolicyBootstrapLAB(t)
	if _, err := ProvisionInstalledManagedNodePolicy(context.Background(), "ir-policy"); err != nil {
		t.Fatal(err)
	}
	b := bindingFixture(t)
	b.NodeID = "ir-policy"
	store, err := OpenReservationStore(privateReservationDir(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReservePending(b); err != nil {
		t.Fatal(err)
	}
	g, err := store.HoldPending(b)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.BindInstalledPolicy(context.Background()); err != nil {
		t.Fatal(err)
	}
	apply := commandIntentFixture(t, b)
	if err := g.RecordCommandIntent(context.Background(), apply); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(store.dir, lifecycleFile)
	original, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*lifecycleEntry){
		func(e *lifecycleEntry) { e.Policy = nil },
		func(e *lifecycleEntry) {
			c := e.Commands[commandIntentApply]
			c.Binding.PlanSHA256 = strings.Repeat("b", 64)
			e.Commands[commandIntentApply] = c
		},
		func(e *lifecycleEntry) {
			c := e.Commands[commandIntentApply]
			c.Action = commandIntentRetire
			e.Commands[commandIntentRetire] = c
		},
		func(e *lifecycleEntry) {
			c := e.Commands[commandIntentApply]
			c.Action = commandIntentRetire
			c.JobID = "orphan-retire"
			e.Commands = map[string]CommandV1{commandIntentRetire: c}
		},
	} {
		var records map[string]lifecycleEntry
		if err := json.Unmarshal(original, &records); err != nil {
			t.Fatal(err)
		}
		entry := records[reservationKey(b)]
		mutate(&entry)
		records[reservationKey(b)] = entry
		raw, err := json.Marshal(records)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, raw, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.InspectCommandIntents(b); err == nil {
			t.Fatal("malformed durable command intent accepted")
		}
	}
}


func TestExpiredRecordedCommandLABPersistsReconciliationWithoutExecution(t *testing.T) {
	isolatedPolicyBootstrapLAB(t)
	ctx := context.Background()
	if _, err := ProvisionInstalledManagedNodePolicy(ctx, "ir-policy"); err != nil {
		t.Fatal(err)
	}
	b := bindingFixture(t)
	b.NodeID = "ir-policy"
	store, err := OpenReservationStore(privateReservationDir(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReservePending(b); err != nil {
		t.Fatal(err)
	}
	g, err := store.HoldPending(b)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.BindInstalledPolicy(ctx); err != nil {
		t.Fatal(err)
	}
	cmd := commandIntentFixture(t, b)
	cmd.IssuedAt = time.Now().Add(-time.Second)
	cmd.ExpiresAt = time.Now().Add(200 * time.Millisecond)
	if err := g.RecordCommandIntent(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	if err := g.MarkApplying(ctx, b); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Until(cmd.ExpiresAt) + 20*time.Millisecond)
	if err := g.CheckHeldCommand(ctx, cmd); err == nil {
		t.Fatal("expired command still eligible for execution")
	}
	if err := g.RecordReconciliationNeeded(ctx, cmd); err != nil {
		t.Fatalf("historically valid expired command cannot fence uncertain outcome: %v", err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenReservationStore(store.dir)
	if err != nil {
		t.Fatal(err)
	}
	state, err := reopened.InspectRecovery(b)
	if err != nil || state != StateNeedsReconciliation {
		t.Fatalf("reopen lost reconciliation fence: %s %v", state, err)
	}
	g, err = reopened.HoldPending(b)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	if err := g.CheckHeldCommand(ctx, cmd); err == nil {
		t.Fatal("reopened expired command granted execution")
	}
	if err := g.RecordReconciliationNeeded(ctx, cmd); err != nil {
		t.Fatalf("identical historical reconciliation retry refused: %v", err)
	}
	changed := cmd
	changed.JobID = "unrecorded-recovery"
	if err := g.RecordReconciliationNeeded(ctx, changed); err == nil {
		t.Fatal("changed immutable command altered reconciliation")
	}
	other := b
	other.LeaseID = "expired-command-other-lease"
	if err := reopened.ReservePending(other); err == nil {
		t.Fatal("expired command released resource claim")
	}
}
