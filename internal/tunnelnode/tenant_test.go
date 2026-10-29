package tunnelnode

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/zarkmakerburg/baft/internal/config"
	"github.com/zarkmakerburg/baft/internal/securityinternal"
)

func TestApplyTenantMutationAddsAndRemovesWithDigestFence(t *testing.T) {
	p := newPair(t)
	p.build(t, "tenant-base")
	for _, n := range []*node{p.ex, p.ir} {
		if _, err := n.Finalize(context.Background(), "tenant-base"); err != nil {
			t.Fatal(err)
		}
	}

	key, err := securityinternal.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := securityinternal.EncodePublicKey(key.Public)
	if err != nil {
		t.Fatal(err)
	}
	readProof := func() (config.Config, string, string, string) {
		t.Helper()
		cfg, err := config.LoadFile(p.ex.liveConfig())
		if err != nil {
			t.Fatal(err)
		}
		cfgBytes, err := os.ReadFile(p.ex.liveConfig())
		if err != nil {
			t.Fatal(err)
		}
		markerBytes, err := os.ReadFile(p.ex.markerPath())
		if err != nil {
			t.Fatal(err)
		}
		unitBytes, err := os.ReadFile(p.ex.unitPath())
		if err != nil {
			t.Fatal(err)
		}
		return cfg, shaHex(cfgBytes), shaHex(unitBytes), shaHex(markerBytes)
	}
	cfg, cfgSHA, unitSHA, markerSHA := readProof()
	if cfg.Server == nil || cfg.Noise == nil {
		t.Fatal("expected a committed EX listener")
	}
	peerID := "urn:baft:node:tenant-test"
	add := TenantMutation{
		ID: "tenant-add-1", Operation: "add", ExpectedOwnerTunnelID: "tenant-base", NodeID: cfg.Node.ID, Listen: cfg.Server.Listen,
		ExpectedConfigSHA256: cfgSHA, ExpectedUnitSHA256: unitSHA, ExpectedMarkerSHA256: markerSHA,
		PeerID: peerID, PeerPublicKey: publicKey, RouteID: p.exp.RouteID, Target: "127.0.0.1:2444",
	}
	p.ex.host.mu.Lock()
	callsBeforeMutation := len(p.ex.host.calls)
	p.ex.host.mu.Unlock()
	ev, err := p.ex.ApplyTenantMutation(context.Background(), add)
	if err != nil {
		t.Fatalf("add tenant: %v", err)
	}
	p.ex.host.mu.Lock()
	mutationCalls := append([]string(nil), p.ex.host.calls[callsBeforeMutation:]...)
	p.ex.host.mu.Unlock()
	sawLiveReload := false
	for _, call := range mutationCalls {
		if strings.HasPrefix(call, "reload ") {
			sawLiveReload = true
		}
		if strings.HasPrefix(call, "restart ") {
			t.Fatalf("tenant add restarted the shared listener, interrupting existing tenants: calls=%v", mutationCalls)
		}
	}
	if !sawLiveReload {
		t.Fatalf("tenant add did not request a live listener reload: calls=%v", mutationCalls)
	}
	retry, err := p.ex.ApplyTenantMutation(context.Background(), add)
	if err != nil || retry != ev {
		t.Fatalf("identical retry should return durable receipt: evidence=%+v err=%v", retry, err)
	}
	cfg, cfgSHA, unitSHA, markerSHA = readProof()
	if cfg.Noise.AllowedPeerPublicKeys[peerID] != publicKey {
		t.Fatal("tenant key not present in live config")
	}
markerBytes, err := os.ReadFile(p.ex.markerPath())
	if err != nil { t.Fatal(err) }
	var markerAfterAdd Marker
	if err := json.Unmarshal(markerBytes, &markerAfterAdd); err != nil { t.Fatal(err) }
	unitBytes, err := os.ReadFile(p.ex.unitPath())
	if err != nil { t.Fatal(err) }
	if markerAfterAdd.Generation != 2 || markerAfterAdd.UnitSHA256 != shaHex(unitBytes) ||
		!strings.Contains(string(unitBytes), "# baft-generation: 2") {
		t.Fatalf("unit and ownership marker generation did not advance together: marker=%+v", markerAfterAdd)
	}
	remove := TenantMutation{
		ID: "tenant-remove-1", Operation: "remove", ExpectedOwnerTunnelID: "tenant-base", NodeID: cfg.Node.ID, Listen: cfg.Server.Listen,
		ExpectedConfigSHA256: cfgSHA, ExpectedUnitSHA256: unitSHA, ExpectedMarkerSHA256: markerSHA,
		PeerID: peerID, PeerPublicKey: publicKey, RouteID: p.exp.RouteID,
	}
	if _, err := p.ex.ApplyTenantMutation(context.Background(), remove); err != nil {
		t.Fatalf("remove tenant: %v", err)
	}
	cfg, _, _, _ = readProof()
	if _, exists := cfg.Noise.AllowedPeerPublicKeys[peerID]; exists {
		t.Fatal("tenant key remained after removal")
	}
}

func TestApplyTenantMutationRejectsStaleDigest(t *testing.T) {
	p := newPair(t)
	p.build(t, "tenant-stale")
	for _, n := range []*node{p.ex, p.ir} {
		if _, err := n.Finalize(context.Background(), "tenant-stale"); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := config.LoadFile(p.ex.liveConfig())
	if err != nil {
		t.Fatal(err)
	}
	markerBytes, err := os.ReadFile(p.ex.markerPath())
	if err != nil {
		t.Fatal(err)
	}
	var marker Marker
	if err := json.Unmarshal(markerBytes, &marker); err != nil {
		t.Fatal(err)
	}
	key, err := securityinternal.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := securityinternal.EncodePublicKey(key.Public)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.ex.ApplyTenantMutation(context.Background(), TenantMutation{
		ID: "tenant-stale-1", Operation: "add", ExpectedOwnerTunnelID: "tenant-stale", NodeID: cfg.Node.ID, Listen: cfg.Server.Listen,
		ExpectedConfigSHA256: "0000000000000000000000000000000000000000000000000000000000000000",
		ExpectedUnitSHA256: shaHex(mustReadTestFile(t, p.ex.unitPath())), ExpectedMarkerSHA256: shaHex(markerBytes), PeerID: "urn:baft:node:tenant-stale",
		PeerPublicKey: publicKey, RouteID: p.exp.RouteID, Target: "127.0.0.1:2444",
	})
	if err == nil {
		t.Fatal("stale config digest must be rejected")
	}
}

func mustReadTestFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil { t.Fatal(err) }
	return b
}


func newFinalizedTenantMutationFixture(t *testing.T, tunnelID, mutationID string) (*pair, TenantMutation, []byte, int) {
	t.Helper()
	p := newPair(t)
	p.build(t, tunnelID)
	for _, n := range []*node{p.ex, p.ir} {
		if _, err := n.Finalize(context.Background(), tunnelID); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := config.LoadFile(p.ex.liveConfig())
	if err != nil {
		t.Fatal(err)
	}
	configBytes, err := os.ReadFile(p.ex.liveConfig())
	if err != nil {
		t.Fatal(err)
	}
	unitBytes, err := os.ReadFile(p.ex.unitPath())
	if err != nil {
		t.Fatal(err)
	}
	markerBytes, err := os.ReadFile(p.ex.markerPath())
	if err != nil {
		t.Fatal(err)
	}
	key, err := securityinternal.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := securityinternal.EncodePublicKey(key.Public)
	if err != nil {
		t.Fatal(err)
	}
	mutation := TenantMutation{
		ID: mutationID, Operation: "add", ExpectedOwnerTunnelID: tunnelID,
		NodeID: cfg.Node.ID, Listen: cfg.Server.Listen,
		ExpectedConfigSHA256: shaHex(configBytes), ExpectedUnitSHA256: shaHex(unitBytes),
		ExpectedMarkerSHA256: shaHex(markerBytes), PeerID: "urn:baft:node:" + mutationID,
		PeerPublicKey: publicKey, RouteID: p.exp.RouteID, Target: "127.0.0.1:2444",
	}
	return p, mutation, configBytes, p.ex.readGeneration()
}

func TestRecoverTenantMutationRejectsGenerationOnlyDrift(t *testing.T) {
	p, mutation, _, _ := newFinalizedTenantMutationFixture(t, "generation-fence", "tenant-generation")
	ev, err := p.ex.ApplyTenantMutation(context.Background(), mutation)
	if err != nil {
		t.Fatalf("apply tenant mutation: %v", err)
	}
	var journal tenantMutationJournal
	if err := readJSON(p.ex.tenantMutationPath(mutation.ID), &journal); err != nil {
		t.Fatal(err)
	}
	journal.Phase = "applying"
	if err := writeJSON(p.ex.tenantMutationPath(mutation.ID), journal, 0o600); err != nil {
		t.Fatal(err)
	}
	configBefore, err := os.ReadFile(p.ex.liveConfig())
	if err != nil {
		t.Fatal(err)
	}
	unitBefore, err := os.ReadFile(p.ex.unitPath())
	if err != nil {
		t.Fatal(err)
	}
	markerBefore, err := os.ReadFile(p.ex.markerPath())
	if err != nil {
		t.Fatal(err)
	}
	ownerTxnBefore, err := os.ReadFile(p.ex.txnPath(mutation.ExpectedOwnerTunnelID))
	if err != nil {
		t.Fatal(err)
	}
	driftGeneration := ev.Generation + 9
	if err := p.ex.writeGeneration(driftGeneration); err != nil {
		t.Fatal(err)
	}
	if err := p.ex.recoverTenantMutations(context.Background()); err == nil || !strings.Contains(err.Error(), "generation drifted") {
		t.Fatalf("generation-only drift must fail closed: %v", err)
	}
	if got := p.ex.readGeneration(); got != driftGeneration {
		t.Fatalf("rollback overwrote unrelated generation: got=%d want=%d", got, driftGeneration)
	}
	for path, want := range map[string][]byte{
		p.ex.liveConfig(): configBefore, p.ex.unitPath(): unitBefore,
		p.ex.markerPath(): markerBefore, p.ex.txnPath(mutation.ExpectedOwnerTunnelID): ownerTxnBefore,
	} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != string(want) {
			t.Fatalf("rollback changed %s despite generation drift: err=%v", path, err)
		}
	}
	if err := readJSON(p.ex.tenantMutationPath(mutation.ID), &journal); err != nil || journal.Phase != "applying" {
		t.Fatalf("unresolved journal should remain available for recovery: journal=%+v err=%v", journal, err)
	}
}

func TestSharedJoinRollbackRetainsClaimUntilTenantJournalRecovers(t *testing.T) {
	p, mutation, oldConfig, oldGeneration := newFinalizedTenantMutationFixture(t, "join-owner", "join-recovery")
	ctx := context.Background()
	if _, err := p.ex.PrepareEXJoin(ctx, mutation.ID, mutation.ExpectedOwnerTunnelID, p.exp); err != nil {
		t.Fatalf("prepare shared join: %v", err)
	}
	if _, err := p.ex.ApplyTenantMutation(ctx, mutation); err != nil {
		t.Fatalf("apply shared tenant mutation: %v", err)
	}
	p.ex.host.failRestart = true
	if _, err := p.ex.Rollback(ctx, mutation.ID); err == nil {
		t.Fatal("faulted tenant rollback unexpectedly succeeded")
	}
	if got := p.ex.activeID(); got != mutation.ID {
		t.Fatalf("failed tenant rollback cleared the logical join claim: active=%q", got)
	}
	txn, err := p.ex.readTxn(mutation.ID)
	if err != nil || txn.Phase != PhasePrepared || txn.ConfigWritten {
		t.Fatalf("logical join was marked rolled back while tenant journal remained unresolved: txn=%+v err=%v", txn, err)
	}
	var journal tenantMutationJournal
	if err := readJSON(p.ex.tenantMutationPath(mutation.ID), &journal); err != nil || journal.Phase != "committed" {
		t.Fatalf("failed rollback should leave the tenant journal unresolved: journal=%+v err=%v", journal, err)
	}
	p.ex.host.failRestart = false
	if _, err := p.ex.Rollback(ctx, mutation.ID); err != nil {
		t.Fatalf("retry shared join rollback: %v", err)
	}
	if got := p.ex.activeID(); got != "" {
		t.Fatalf("successful recovery left active join claim %q", got)
	}
	txn, err = p.ex.readTxn(mutation.ID)
	if err != nil || txn.Phase != PhaseRolledBack {
		t.Fatalf("logical join did not finish rollback after journal recovery: txn=%+v err=%v", txn, err)
	}
	if err := readJSON(p.ex.tenantMutationPath(mutation.ID), &journal); err != nil || journal.Phase != "rolled_back" {
		t.Fatalf("tenant journal did not record recovered rollback: journal=%+v err=%v", journal, err)
	}
	configAfter, err := os.ReadFile(p.ex.liveConfig())
	if err != nil || string(configAfter) != string(oldConfig) {
		t.Fatalf("tenant rollback did not restore the owner config: err=%v", err)
	}
	if got := p.ex.readGeneration(); got != oldGeneration {
		t.Fatalf("tenant rollback generation=%d want=%d", got, oldGeneration)
	}
}


func markSharedJoinCommittedForTest(t *testing.T, p *pair, mutation TenantMutation, evidence TenantMutationEvidence) {
	t.Helper()
	for suffix, destination := range map[string]string{
		"old-config": p.ex.backup(mutation.ID, "baft.yaml"),
		"old-unit": p.ex.backup(mutation.ID, "unit"),
		"old-marker": p.ex.backup(mutation.ID, "marker.json"),
		"old-owner-txn": p.ex.backup(mutation.ID, "owner-txn.json"),
	} {
		b, err := os.ReadFile(p.ex.tenantMutationBlob(mutation.ID, suffix))
		if err != nil {
			t.Fatal(err)
		}
		if err := writeFile(destination, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	txn, err := p.ex.readTxn(mutation.ID)
	if err != nil {
		t.Fatal(err)
	}
	txn.HadConfig, txn.HadUnit, txn.HadMarker = true, true, true
	txn.WasActive, txn.WasEnabled = true, true
	txn.Generation, txn.PreviousGeneration = evidence.Generation, evidence.Generation-1
	txn.InstalledConfigSHA, txn.InstalledUnitSHA, txn.InstalledMarkerSHA = evidence.AfterConfigSHA, evidence.AfterUnitSHA, evidence.AfterMarkerSHA
	txn.ConfigWritten, txn.UnitChanged = true, true
	txn.PeerID, txn.PeerPublicKey = mutation.PeerID, mutation.PeerPublicKey
	txn.TenantRouteID, txn.TenantTarget = mutation.RouteID, mutation.Target
	txn.Phase = PhaseCommitted
	if err := p.ex.writeTxn(txn); err != nil {
		t.Fatal(err)
	}
}

func TestCommittedSharedJoinRollbackRecoversJournalAndPreservesClaim(t *testing.T) {
	p, mutation, oldConfig, oldGeneration := newFinalizedTenantMutationFixture(t, "committed-join-owner", "committed-join")
	ctx := context.Background()
	if _, err := p.ex.PrepareEXJoin(ctx, mutation.ID, mutation.ExpectedOwnerTunnelID, p.exp); err != nil {
		t.Fatalf("prepare shared join: %v", err)
	}
	oldUnit, err := os.ReadFile(p.ex.unitPath())
	if err != nil {
		t.Fatal(err)
	}
	oldMarker, err := os.ReadFile(p.ex.markerPath())
	if err != nil {
		t.Fatal(err)
	}
	oldOwnerTxn, err := os.ReadFile(p.ex.txnPath(mutation.ExpectedOwnerTunnelID))
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := p.ex.ApplyTenantMutation(ctx, mutation)
	if err != nil {
		t.Fatalf("apply shared tenant mutation: %v", err)
	}
	markSharedJoinCommittedForTest(t, p, mutation, evidence)

	p.ex.host.failRestart = true
	if _, err := p.ex.Rollback(ctx, mutation.ID); err == nil {
		t.Fatal("faulted committed-join rollback unexpectedly succeeded")
	}
	if got := p.ex.activeID(); got != mutation.ID {
		t.Fatalf("failed rollback cleared the logical join claim: active=%q", got)
	}
	txn, err := p.ex.readTxn(mutation.ID)
	if err != nil || txn.Phase != PhaseCommitted || !txn.ConfigWritten {
		t.Fatalf("failed rollback changed the committed join state: txn=%+v err=%v", txn, err)
	}
	var journal tenantMutationJournal
	if err := readJSON(p.ex.tenantMutationPath(mutation.ID), &journal); err != nil || journal.Phase != "committed" {
		t.Fatalf("failed rollback should leave journal recoverable: journal=%+v err=%v", journal, err)
	}
	partialConfig, err := os.ReadFile(p.ex.liveConfig())
	if err != nil || string(partialConfig) != string(oldConfig) {
		t.Fatalf("journal rollback did not restore the tenant config before service failure: err=%v", err)
	}

	p.ex.host.failRestart = false
	if _, err := p.ex.Rollback(ctx, mutation.ID); err != nil {
		t.Fatalf("retry committed shared-join rollback: %v", err)
	}
	if got := p.ex.activeID(); got != "" {
		t.Fatalf("successful retry left active join claim %q", got)
	}
	txn, err = p.ex.readTxn(mutation.ID)
	if err != nil || txn.Phase != PhaseRolledBack {
		t.Fatalf("logical join did not finish rollback: txn=%+v err=%v", txn, err)
	}
	if err := readJSON(p.ex.tenantMutationPath(mutation.ID), &journal); err != nil || journal.Phase != "rolled_back" {
		t.Fatalf("tenant journal did not record successful rollback: journal=%+v err=%v", journal, err)
	}
	configAfter, err := os.ReadFile(p.ex.liveConfig())
	if err != nil || string(configAfter) != string(oldConfig) {
		t.Fatalf("rollback changed other owner tenants or config: err=%v", err)
	}
	cfgAfter, err := config.LoadFile(p.ex.liveConfig())
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := cfgAfter.Noise.AllowedPeerPublicKeys[mutation.PeerID]; exists {
		t.Fatal("rolled-back peer remained on the owner listener")
	}
	if got := p.ex.readGeneration(); got != oldGeneration {
		t.Fatalf("generation=%d want=%d", got, oldGeneration)
	}
	for path, want := range map[string][]byte{
		p.ex.unitPath(): oldUnit, p.ex.markerPath(): oldMarker,
		p.ex.txnPath(mutation.ExpectedOwnerTunnelID): oldOwnerTxn,
	} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != string(want) {
			t.Fatalf("rollback did not restore %s: err=%v", path, err)
		}
	}
	if !p.ex.host.active {
		t.Fatal("owner service was not active after successful journal recovery")
	}
}

func TestCommittedSharedJoinRollbackFailsClosedWithoutJournal(t *testing.T) {
	p, mutation, oldConfig, _ := newFinalizedTenantMutationFixture(t, "missing-journal-owner", "missing-join-journal")
	if _, err := p.ex.PrepareEXJoin(context.Background(), mutation.ID, mutation.ExpectedOwnerTunnelID, p.exp); err != nil {
		t.Fatalf("prepare shared join: %v", err)
	}
	txn, err := p.ex.readTxn(mutation.ID)
	if err != nil {
		t.Fatal(err)
	}
	txn.ConfigWritten, txn.Phase = true, PhaseCommitted
	if err := p.ex.writeTxn(txn); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ex.Rollback(context.Background(), mutation.ID); err == nil || !strings.Contains(err.Error(), "no tenant mutation journal") {
		t.Fatalf("committed shared join without a journal must fail closed: %v", err)
	}
	if got := p.ex.activeID(); got != mutation.ID {
		t.Fatalf("fail-closed rollback cleared active claim: %q", got)
	}
	configAfter, err := os.ReadFile(p.ex.liveConfig())
	if err != nil || string(configAfter) != string(oldConfig) {
		t.Fatalf("fail-closed rollback changed owner config: err=%v", err)
	}
	txn, err = p.ex.readTxn(mutation.ID)
	if err != nil || txn.Phase != PhaseCommitted || !txn.ConfigWritten {
		t.Fatalf("fail-closed rollback changed logical join state: txn=%+v err=%v", txn, err)
	}
}


func TestSharedJoinAddsNewPeerWithoutChangingLegacyOwner(t *testing.T) {
	p := newPair(t)
	ctx := context.Background()
	p.build(t, "shared-owner")
	for _, n := range []*node{p.ex, p.ir} {
		if _, err := n.Finalize(ctx, "shared-owner"); err != nil {
			t.Fatal(err)
		}
	}
	before, err := config.LoadFile(p.ex.liveConfig())
	if err != nil {
		t.Fatal(err)
	}
	if before.Noise == nil || before.Noise.PeerPublicKey == "" || len(before.Server.AllowedPeerIdentities) != 1 {
		t.Fatalf("fixture should begin with a legacy single-peer listener: %+v", before.Noise)
	}

	ir2 := newNode(t)
	ir2Params := IRParams{RouteListen: "127.0.0.1:" + strconv.Itoa(freePort(t)), RouteID: "tenant-route"}
	joinParams := p.exp
	joinParams.Target = "127.0.0.1:2444"
	joinParams.RouteID = "tenant-route"
	code, err := p.ex.PrepareEXJoin(ctx, "shared-tenant", "shared-owner", joinParams)
	if err != nil {
		t.Fatalf("prepare shared tenant: %v", err)
	}
	reply, err := ir2.PrepareIR(ctx, "shared-tenant", code, ir2Params)
	if err != nil {
		t.Fatalf("prepare new IR: %v", err)
	}
	ir2Staged, err := config.LoadFile(filepath.Join(ir2.stage("shared-tenant"), "baft.new.yaml"))
	if err != nil {
		t.Fatalf("read new IR staged config: %v", err)
	}
	ownerPeerID := before.Server.AllowedPeerIdentities[0]
	newPeerID := "urn:baft:node:" + ir2Staged.Node.ID
	p.ex.host.mu.Lock()
	callsBeforeCommit := len(p.ex.host.calls)
	p.ex.host.mu.Unlock()
	detail, receipt, err := p.ex.CommitEXJoin(ctx, "shared-tenant", reply)
	if err != nil {
		t.Fatalf("commit shared tenant: %v", err)
	}
	if detail == "" || receipt == "" {
		t.Fatalf("shared tenant commit omitted evidence: detail=%q receipt=%q", detail, receipt)
	}
	p.ex.host.mu.Lock()
	calls := append([]string(nil), p.ex.host.calls[callsBeforeCommit:]...)
	p.ex.host.mu.Unlock()
	sawReload := false
	for _, call := range calls {
		if strings.HasPrefix(call, "reload ") {
			sawReload = true
		}
		if strings.HasPrefix(call, "restart ") {
			t.Fatalf("shared join restarted the physical listener: %v", calls)
		}
	}
	if !sawReload {
		t.Fatalf("shared join did not hot-reload the physical listener: %v", calls)
	}

	after, err := config.LoadFile(p.ex.liveConfig())
	if err != nil {
		t.Fatal(err)
	}
	if after.Noise == nil || after.Noise.PeerPublicKey != "" ||
		after.Noise.AllowedPeerPublicKeys[ownerPeerID] == "" ||
		after.Noise.AllowedPeerPublicKeys[newPeerID] == "" {
		t.Fatalf("legacy owner peer was not preserved alongside new tenant: %+v", after.Noise)
	}
	if len(after.Server.AllowedPeerIdentities) != 2 {
		t.Fatalf("expected exactly two authenticated peers, got %v", after.Server.AllowedPeerIdentities)
	}
	routesByID := map[string]config.Route{}
	for _, route := range after.Routes {
		routesByID[route.ID] = route
	}
	if routesByID["service-main"].Target != p.exp.Target ||
		len(routesByID["service-main"].AllowedPeers) != 1 ||
		routesByID["service-main"].AllowedPeers[0] != ownerPeerID {
		t.Fatalf("existing owner's route changed: %+v", routesByID["service-main"])
	}
	if routesByID["tenant-route"].Target != joinParams.Target ||
		len(routesByID["tenant-route"].AllowedPeers) != 1 ||
		routesByID["tenant-route"].AllowedPeers[0] != newPeerID {
		t.Fatalf("new tenant route is not isolated: %+v", routesByID["tenant-route"])
	}
	if _, err := ir2.CommitIR(ctx, "shared-tenant"); err != nil {
		t.Fatalf("commit new IR: %v", err)
	}
	if _, err := p.ex.Finalize(ctx, "shared-tenant"); err != nil {
		t.Fatalf("finalize shared tenant: %v", err)
	}
	if _, err := ir2.Finalize(ctx, "shared-tenant"); err != nil {
		t.Fatalf("finalize new IR: %v", err)
	}
}
