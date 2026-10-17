package tunnelengine

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func inventoryFixture(node string) (InventoryChallenge, InventoryReport, time.Time) {
	now := time.Date(2026, 10, 17, 6, 0, 0, 0, time.UTC)
	challenge := InventoryChallenge{NodeID: node, JobID: "inventory-1", Generation: 1, Nonce: strings.Repeat("a", 64), IssuedAt: now, ExpiresAt: now.Add(MaxInventoryLifetime)}
	report := InventoryReport{SchemaVersion: InventorySchemaVersion, NodeID: node, JobID: challenge.JobID, Generation: challenge.Generation, Nonce: challenge.Nonce,
		Engines: []EngineObservation{{Engine: "baft", ContractVersion: DriverContractVersion, Installation: InstallationPresent, Version: "v0.3.3", ArtifactSHA256: strings.Repeat("b", 64)}}}
	return challenge, report, now.Add(time.Second)
}

func inventoryJSON(t *testing.T, report InventoryReport) []byte {
	t.Helper()
	payload, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func inventorySnapshot(t *testing.T, node string) (InventorySnapshot, NodeEngineRequirement, time.Time) {
	t.Helper()
	challenge, report, now := inventoryFixture(node)
	snapshot, err := AdvanceInventory(InventorySnapshot{}, challenge, inventoryJSON(t, report), now)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot, NodeEngineRequirement{NodeID: node, Generation: challenge.Generation, Version: "v0.3.3", SHA256: report.Engines[0].ArtifactSHA256}, now
}

func TestInventoryBindingAndFreshness(t *testing.T) {
	challenge, report, now := inventoryFixture("ir-1")
	// A late but valid receipt gets the original deadline, not a renewed TTL.
	now = challenge.ExpiresAt.Add(-time.Second)
	snapshot, err := AdvanceInventory(InventorySnapshot{}, challenge, inventoryJSON(t, report), now)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.NodeID() != "ir-1" || snapshot.Generation() != 1 || snapshot.ObservedAt() != now || snapshot.ExpiresAt() != challenge.ExpiresAt {
		t.Fatalf("incorrect snapshot metadata: %+v", snapshot)
	}
	if !snapshot.Fresh(now) || snapshot.Fresh(now.Add(-time.Nanosecond)) || snapshot.Fresh(challenge.ExpiresAt) || (InventorySnapshot{}).Fresh(now) {
		t.Fatal("freshness accepted a zero, expired, or clock-rollback observation")
	}
	observation, ok := snapshot.Observation("baft")
	if !ok || observation.Installation != InstallationPresent {
		t.Fatal("native installation observation missing")
	}
	observation.ArtifactSHA256 = "changed by caller"
	actual, _ := snapshot.Observation("baft")
	if actual.ArtifactSHA256 != report.Engines[0].ArtifactSHA256 {
		t.Fatal("caller mutated immutable snapshot")
	}
}

func TestInventoryRejectsUnboundAndContradictoryReports(t *testing.T) {
	cases := map[string]func(*InventoryReport){
		"wrong schema":        func(r *InventoryReport) { r.SchemaVersion++ },
		"wrong node":          func(r *InventoryReport) { r.NodeID = "ex-1" },
		"wrong job":           func(r *InventoryReport) { r.JobID = "another-job" },
		"wrong generation":    func(r *InventoryReport) { r.Generation++ },
		"wrong nonce":         func(r *InventoryReport) { r.Nonce = strings.Repeat("c", 64) },
		"unsupported driver":  func(r *InventoryReport) { r.Engines[0].Engine = "frp" },
		"carrier as driver":   func(r *InventoryReport) { r.Engines[0].Engine = "wss" },
		"unknown driver":      func(r *InventoryReport) { r.Engines[0].Engine = "unregistered" },
		"implicit alias":      func(r *InventoryReport) { r.Engines[0].Engine = "" },
		"driver version":      func(r *InventoryReport) { r.Engines[0].ContractVersion++ },
		"unknown state":       func(r *InventoryReport) { r.Engines[0].Installation = "ready" },
		"missing version":     func(r *InventoryReport) { r.Engines[0].Version = "" },
		"malformed version":   func(r *InventoryReport) { r.Engines[0].Version = "v0.3.3\nsecret" },
		"long version":        func(r *InventoryReport) { r.Engines[0].Version = "v0.3.3-" + strings.Repeat("x", 128) },
		"missing digest":      func(r *InventoryReport) { r.Engines[0].ArtifactSHA256 = "" },
		"malformed digest":    func(r *InventoryReport) { r.Engines[0].ArtifactSHA256 = strings.Repeat("z", 64) },
		"absent with claims":  func(r *InventoryReport) { r.Engines[0].Installation = InstallationAbsent },
		"unknown with claims": func(r *InventoryReport) { r.Engines[0].Installation = InstallationUnknown },
		"duplicate engine":    func(r *InventoryReport) { r.Engines = append(r.Engines, r.Engines[0]) },
		"too many engines":    func(r *InventoryReport) { r.Engines = make([]EngineObservation, MaxInventoryEntries+1) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			challenge, report, now := inventoryFixture("ir-1")
			mutate(&report)
			result, err := AdvanceInventory(InventorySnapshot{}, challenge, inventoryJSON(t, report), now)
			if err == nil || result.Fresh(now) || result.NodeID() != "" {
				t.Fatalf("invalid report produced usable evidence: %v", err)
			}
		})
	}
}

func TestInventoryChallengeAndPayloadBounds(t *testing.T) {
	cases := map[string]func(*InventoryChallenge, *time.Time){
		"empty node":      func(c *InventoryChallenge, _ *time.Time) { c.NodeID = "" },
		"invalid job":     func(c *InventoryChallenge, _ *time.Time) { c.JobID = "../job" },
		"zero generation": func(c *InventoryChallenge, _ *time.Time) { c.Generation = 0 },
		"weak nonce":      func(c *InventoryChallenge, _ *time.Time) { c.Nonce = "a" },
		"zero issued":     func(c *InventoryChallenge, _ *time.Time) { c.IssuedAt = time.Time{} },
		"zero receipt":    func(_ *InventoryChallenge, now *time.Time) { *now = time.Time{} },
		"zero expiry":     func(c *InventoryChallenge, _ *time.Time) { c.ExpiresAt = time.Time{} },
		"inverted window": func(c *InventoryChallenge, _ *time.Time) { c.ExpiresAt = c.IssuedAt },
		"unbounded window": func(c *InventoryChallenge, _ *time.Time) {
			c.ExpiresAt = c.IssuedAt.Add(MaxInventoryLifetime + time.Nanosecond)
		},
		"future issue":    func(c *InventoryChallenge, now *time.Time) { *now = c.IssuedAt.Add(-time.Nanosecond) },
		"expired receipt": func(c *InventoryChallenge, now *time.Time) { *now = c.ExpiresAt },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			challenge, report, now := inventoryFixture("ir-1")
			mutate(&challenge, &now)
			report.NodeID, report.JobID, report.Generation, report.Nonce = challenge.NodeID, challenge.JobID, challenge.Generation, challenge.Nonce
			if _, err := AdvanceInventory(InventorySnapshot{}, challenge, inventoryJSON(t, report), now); err == nil {
				t.Fatal("invalid challenge accepted")
			}
		})
	}
	challenge, report, now := inventoryFixture("ir-1")
	valid := string(inventoryJSON(t, report))
	for name, payload := range map[string]string{
		"empty": "", "oversize": strings.Repeat(" ", MaxInventoryBytes+1), "syntax": "{", "trailing object": valid + "{}", "trailing null": valid + "null", "trailing garbage": valid + "!",
		"caller verified flag": strings.TrimSuffix(valid, "}") + `,"verified":true}`,
		"agent timestamp":      strings.TrimSuffix(valid, "}") + `,"observed_at":"2099-01-01T00:00:00Z"}`,
		"caller ready flag":    strings.Replace(valid, `"installation":"present"`, `"installation":"present","ready":true`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if result, err := AdvanceInventory(InventorySnapshot{}, challenge, []byte(payload), now); err == nil || result.Fresh(now) {
				t.Fatal("malformed or self-attesting payload accepted")
			}
		})
	}
}

func TestInventoryAdvanceDoesNotRenewReplaysOrRegressState(t *testing.T) {
	challenge, report, now := inventoryFixture("ir-1")
	previous, err := AdvanceInventory(InventorySnapshot{}, challenge, inventoryJSON(t, report), now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AdvanceInventory(previous, challenge, inventoryJSON(t, report), now.Add(time.Second)); err == nil {
		t.Fatal("replayed report renewed evidence")
	}
	next := challenge
	next.Generation, next.JobID, next.Nonce = 2, "inventory-2", strings.Repeat("c", 64)
	next.IssuedAt, next.ExpiresAt = now.Add(time.Second), now.Add(time.Minute)
	report.Generation, report.JobID, report.Nonce = next.Generation, next.JobID, next.Nonce
	advanced, err := AdvanceInventory(previous, next, inventoryJSON(t, report), now.Add(2*time.Second))
	if err != nil || advanced.Generation() != 2 || previous.Generation() != 1 {
		t.Fatalf("monotonic advance failed or mutated previous: %v", err)
	}
	for name, mutate := range map[string]func(*InventoryChallenge){
		"same job":     func(c *InventoryChallenge) { c.JobID = challenge.JobID },
		"older issue":  func(c *InventoryChallenge) { c.IssuedAt = challenge.IssuedAt.Add(-time.Second) },
		"other node":   func(c *InventoryChallenge) { c.NodeID = "ex-1" },
		"reused nonce": func(c *InventoryChallenge) { c.Nonce = challenge.Nonce },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := next
			mutate(&candidate)
			matching := report
			matching.NodeID, matching.JobID, matching.Generation, matching.Nonce = candidate.NodeID, candidate.JobID, candidate.Generation, candidate.Nonce
			if _, err := AdvanceInventory(previous, candidate, inventoryJSON(t, matching), now.Add(2*time.Second)); err == nil {
				t.Fatal("invalid update accepted")
			}
			if !previous.Fresh(now) || previous.Generation() != 1 {
				t.Fatal("failed update changed previous evidence")
			}
		})
	}
	_, original, _ := inventoryFixture("ir-1")
	if _, err := AdvanceInventory(advanced, challenge, inventoryJSON(t, original), now.Add(3*time.Second)); err == nil {
		t.Fatal("older generation overwrote newer evidence")
	}
}

func TestPairInstalledAdmissionIsConservative(t *testing.T) {
	ir, wantIR, now := inventorySnapshot(t, "ir-1")
	ex, wantEX, _ := inventorySnapshot(t, "ex-1")
	selection := EngineSelection{Engine: "baft", ContractVersion: DriverContractVersion, Direction: "ir_to_ex", Network: "tcp"}
	if err := RequirePairInstalled(selection, ir, ex, wantIR, wantEX, now); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*EngineSelection, *InventorySnapshot, *InventorySnapshot, *NodeEngineRequirement, *NodeEngineRequirement, *time.Time){
		"unsupported FRP": func(s *EngineSelection, _, _ *InventorySnapshot, _, _ *NodeEngineRequirement, _ *time.Time) {
			s.Engine = "frp"
		},
		"empty alias": func(s *EngineSelection, _, _ *InventorySnapshot, _, _ *NodeEngineRequirement, _ *time.Time) {
			s.Engine = ""
		},
		"schema downgrade": func(s *EngineSelection, _, _ *InventorySnapshot, _, _ *NodeEngineRequirement, _ *time.Time) {
			s.ContractVersion = 0
		},
		"future contract": func(s *EngineSelection, _, _ *InventorySnapshot, _, _ *NodeEngineRequirement, _ *time.Time) {
			s.ContractVersion++
		},
		"reverse": func(s *EngineSelection, _, _ *InventorySnapshot, _, _ *NodeEngineRequirement, _ *time.Time) {
			s.Direction = "ex_to_ir"
		},
		"UDP": func(s *EngineSelection, _, _ *InventorySnapshot, _, _ *NodeEngineRequirement, _ *time.Time) {
			s.Network = "udp"
		},
		"missing IR": func(_ *EngineSelection, i, _ *InventorySnapshot, _, _ *NodeEngineRequirement, _ *time.Time) {
			*i = InventorySnapshot{}
		},
		"wrong node": func(_ *EngineSelection, _, _ *InventorySnapshot, i, _ *NodeEngineRequirement, _ *time.Time) {
			i.NodeID = "another-node"
		},
		"same endpoints": func(_ *EngineSelection, _, _ *InventorySnapshot, _, e *NodeEngineRequirement, _ *time.Time) {
			e.NodeID = wantIR.NodeID
		},
		"untrusted digest": func(_ *EngineSelection, _, _ *InventorySnapshot, _, e *NodeEngineRequirement, _ *time.Time) {
			e.SHA256 = strings.Repeat("c", 64)
		},
		"untrusted version": func(_ *EngineSelection, _, _ *InventorySnapshot, _, e *NodeEngineRequirement, _ *time.Time) {
			e.Version = "v0.3.4"
		},
		"missing artifact policy": func(_ *EngineSelection, _, _ *InventorySnapshot, _, e *NodeEngineRequirement, _ *time.Time) {
			e.SHA256 = ""
		},
		"newer probe pending": func(_ *EngineSelection, _, _ *InventorySnapshot, _, e *NodeEngineRequirement, _ *time.Time) {
			e.Generation++
		},
		"missing probe generation": func(_ *EngineSelection, _, _ *InventorySnapshot, _, e *NodeEngineRequirement, _ *time.Time) {
			e.Generation = 0
		},
		"expired": func(_ *EngineSelection, _, _ *InventorySnapshot, _, _ *NodeEngineRequirement, n *time.Time) {
			*n = ex.ExpiresAt()
		},
		"clock rollback": func(_ *EngineSelection, _, _ *InventorySnapshot, _, _ *NodeEngineRequirement, n *time.Time) {
			*n = now.Add(-time.Nanosecond)
		},
	} {
		t.Run(name, func(t *testing.T) {
			s, i, e, wi, we, n := selection, ir, ex, wantIR, wantEX, now
			change(&s, &i, &e, &wi, &we, &n)
			if err := RequirePairInstalled(s, i, e, wi, we, n); err == nil {
				t.Fatal("unproven pair admitted")
			}
		})
	}
	for _, state := range []Installation{InstallationUnknown, InstallationAbsent} {
		challenge, report, received := inventoryFixture("ex-1")
		report.Engines[0] = EngineObservation{Engine: "baft", ContractVersion: DriverContractVersion, Installation: state}
		snapshot, err := AdvanceInventory(InventorySnapshot{}, challenge, inventoryJSON(t, report), received)
		if err != nil {
			t.Fatal(err)
		}
		if err := RequirePairInstalled(selection, ir, snapshot, wantIR, wantEX, now); err == nil {
			t.Fatalf("%s installation admitted", state)
		}
	}
}

func TestPairMayUseDifferentVerifiedArtifactsPerNode(t *testing.T) {
	ir, wantIR, now := inventorySnapshot(t, "ir-1")
	challenge, report, _ := inventoryFixture("ex-1")
	report.Engines[0].ArtifactSHA256 = strings.Repeat("c", 64)
	ex, err := AdvanceInventory(InventorySnapshot{}, challenge, inventoryJSON(t, report), now)
	if err != nil {
		t.Fatal(err)
	}
	wantEX := NodeEngineRequirement{NodeID: "ex-1", Generation: 1, Version: "v0.3.3", SHA256: report.Engines[0].ArtifactSHA256}
	selection := EngineSelection{Engine: "baft", ContractVersion: DriverContractVersion, Direction: "ir_to_ex", Network: "tcp"}
	if err := RequirePairInstalled(selection, ir, ex, wantIR, wantEX, now); err != nil {
		t.Fatalf("per-node artifact digests were incorrectly conflated: %v", err)
	}
}

func TestMissingEngineAndUntrustedSnapshotJSONCannotSupplyEvidence(t *testing.T) {
	ir, wantIR, now := inventorySnapshot(t, "ir-1")
	challenge, report, _ := inventoryFixture("ex-1")
	wantEX := NodeEngineRequirement{NodeID: "ex-1", Generation: 1, Version: "v0.3.3", SHA256: report.Engines[0].ArtifactSHA256}
	report.Engines = nil
	empty, err := AdvanceInventory(InventorySnapshot{}, challenge, inventoryJSON(t, report), now)
	if err != nil {
		t.Fatal(err)
	}
	selection := EngineSelection{Engine: "baft", ContractVersion: DriverContractVersion, Direction: "ir_to_ex", Network: "tcp"}
	if err := RequirePairInstalled(selection, ir, empty, wantIR, wantEX, now); err == nil {
		t.Fatal("an empty but fresh report supplied installed-engine evidence")
	}
	var forged InventorySnapshot
	if err := json.Unmarshal([]byte(`{"node_id":"ex-1","generation":1,"ready":true,"verified":true}`), &forged); err != nil {
		t.Fatal(err)
	}
	if forged.Fresh(now) || forged.NodeID() != "" {
		t.Fatal("untrusted snapshot JSON bypassed validation")
	}
}

func TestInventoryExactPayloadSizeBoundary(t *testing.T) {
	challenge, report, now := inventoryFixture("ir-1")
	payload := inventoryJSON(t, report)
	payload = append(payload, []byte(strings.Repeat(" ", MaxInventoryBytes-len(payload)))...)
	if _, err := AdvanceInventory(InventorySnapshot{}, challenge, payload, now); err != nil {
		t.Fatalf("valid report at size boundary rejected: %v", err)
	}
	if _, err := AdvanceInventory(InventorySnapshot{}, challenge, append(payload, ' '), now); err == nil {
		t.Fatal("report exceeding size boundary accepted")
	}
}

func TestInventorySnapshotConcurrentReads(t *testing.T) {
	snapshot, _, now := inventorySnapshot(t, "ir-1")
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := 0; j < 100; j++ {
				observation, ok := snapshot.Observation("baft")
				if !ok || observation.Version != "v0.3.3" || !snapshot.Fresh(now) {
					t.Error("immutable snapshot changed during concurrent reads")
					return
				}
			}
		}()
	}
	workers.Wait()
}
