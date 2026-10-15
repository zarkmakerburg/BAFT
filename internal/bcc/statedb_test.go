package bcc

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/telemetry"
	"github.com/zarkmakerburg/baft/internal/incidentclass"
)

func populatedState(t *testing.T, path string) *Store {
	t.Helper()
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"ex-1", "ex-2"} {
		if _, err := store.UpsertNode(Node{ID: id, Alias: "EX " + id, Address: "127.0.0.1:1", Role: "foreign"}, "tok-"+id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.UpsertNode(Node{ID: "w-1", Alias: "worker", Address: "127.0.0.1:2", Role: "worker", PublicKey: "pk"}, "tok-w"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateDeployJobs([]string{"ex-1", "ex-2"}, "v1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetFinancePolicyAt("ex-1", 1000, 3000, "IRR", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddTraffic("ex-1", "tok-ex-1", 3<<30, 1<<30); err != nil {
		t.Fatal(err)
	}
	if err := store.SetActiveAlerts(map[string]Alert{"stale/ex-2": {Type: "telemetry_stale", Status: "firing", NodeID: "ex-2"}}); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	store.st.History["ex-1"] = []HistoryPoint{{Timestamp: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), IngressBytes: 7, Routes: []telemetry.RouteSnapshot{{RouteID: "r"}}}}
	store.st.RetiredBootIDs["ex-1"] = map[string]bool{"boot-a": true}
	store.st.NextTelemetryIngestID = 42
	err = store.saveLocked()
	store.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestStateIsASQLiteDatabaseAndRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store := populatedState(t, path)
	raw, err := os.ReadFile(path)
	if err != nil || !isSQLiteFile(raw) {
		t.Fatalf("state file is not SQLite: %v", err)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Fatalf("state file mode %o", st.Mode().Perm())
	}
	if v, err := stateSchemaVersion(path); err != nil || v != len(stateMigrations) {
		t.Fatalf("schema version %d, %v", v, err)
	}
	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(canonicalStateJSON(store.st), canonicalStateJSON(reopened.st)) {
		t.Fatalf("state changed across reopen\nwant %s\ngot  %s", canonicalStateJSON(store.st), canonicalStateJSON(reopened.st))
	}
	if reopened.st.NextTelemetryIngestID != 42 || len(reopened.st.FinanceLedger) == 0 || !reopened.st.RetiredBootIDs["ex-1"]["boot-a"] {
		t.Fatal("counters, ledger or retired boot IDs were lost")
	}
}

func TestJSONStateIsMigratedInPlace(t *testing.T) {
	dir := t.TempDir()
	src := populatedState(t, filepath.Join(dir, "src.json"))
	legacy, err := json.MarshalIndent(src.st, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if !isSQLiteFile(raw) {
		t.Fatal("JSON state was not converted")
	}
	bak, err := os.ReadFile(path + ".json.bak")
	if err != nil || !bytes.Equal(bak, legacy) {
		t.Fatalf("original JSON not kept: %v", err)
	}
	if !bytes.Equal(canonicalStateJSON(src.st), canonicalStateJSON(store.st)) {
		t.Fatal("migrated state differs from the JSON state")
	}
	if _, err := os.Stat(path + ".sqlite-migrating"); !os.IsNotExist(err) {
		t.Fatal("migration left its temporary database")
	}
	// Agents keep authenticating with their existing tokens.
	if _, err := store.PullJobs("ex-1", "tok-ex-1"); err != nil {
		t.Fatalf("agent token lost in migration: %v", err)
	}
}

func TestLegacyJSONWithoutRateHistoryIsUpgraded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	legacy := `{"nodes":{},"jobs":{},"finance_policies":{"ex-1":{"node_id":"ex-1","cost_micros_per_gib":5,"revenue_micros_per_gib":9}},"next_job":3}`
	os.WriteFile(path, []byte(legacy), 0o600)
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	h := store.st.RateHistory["ex-1"]
	if len(h) != 1 || h[0].Currency != "IRR" || h[0].Version == 0 || store.st.NextJob != 3 {
		t.Fatalf("legacy upgrade lost data: %+v next_job=%d", h, store.st.NextJob)
	}
}

func TestNewerSchemaIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	populatedState(t, path)
	db, err := openStateDB(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, 'future')`, len(stateMigrations)+1); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := OpenStore(path); err == nil || !strings.Contains(err.Error(), "newer than this build") {
		t.Fatalf("newer schema accepted: %v", err)
	}
}

func TestCorruptStateDatabaseIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	populatedState(t, path)
	raw, _ := os.ReadFile(path)
	for i := 100; i < len(raw); i++ {
		raw[i] = 0xff
	}
	os.WriteFile(path, raw, 0o600)
	if _, err := OpenStore(path); err == nil {
		t.Fatal("corrupt database accepted")
	}
}

func TestLedgerAppendsAndRewritesStayExact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	var st state
	normalizeState(&st)
	entry := func(i int) FinanceLedgerEntry {
		return FinanceLedgerEntry{NodeID: "ex-1", Timestamp: time.Unix(int64(i), 0).UTC(), IngressBytes: uint64(i), Currency: "IRR"}
	}
	check := func(what string) {
		t.Helper()
		got, err := readStateDB(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(canonicalStateJSON(got), canonicalStateJSON(st)) {
			t.Fatalf("%s: stored ledger %d entries, want %d", what, len(got.FinanceLedger), len(st.FinanceLedger))
		}
	}
	for i := 0; i < 5; i++ {
		st.FinanceLedger = append(st.FinanceLedger, entry(i))
		if err := writeStateDB(path, st); err != nil {
			t.Fatal(err)
		}
	}
	check("appends")
	// Same length, different content: must not be mistaken for a prefix.
	st.FinanceLedger[4] = entry(99)
	writeStateDB(path, st)
	check("replaced last entry")
	// Shorter ledger (as after a restore): full rewrite.
	st.FinanceLedger = st.FinanceLedger[:2]
	writeStateDB(path, st)
	check("shrunk")
	st.FinanceLedger = nil
	writeStateDB(path, st)
	check("emptied")
}


// v0.2.0 (tag 4d806e01) had precisely the first five state migrations.
// Exercise the on-disk upgrade path with data, rather than opening an empty
// current-schema database. This is an upgrade check, not a downgrade promise.
func TestReleasedV020SchemaUpgradesWithoutLosingNode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := sql.Open("sqlite", path)
	if err != nil { t.Fatal(err) }
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil { t.Fatal(err) }
	for i := 0; i < 5; i++ {
		if _, err := db.Exec(stateMigrations[i]); err != nil { t.Fatalf("v0.2.0 migration %d: %v", i+1, err) }
		if _, err := db.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (?, 'v0.2.0')`, i+1); err != nil { t.Fatal(err) }
	}
	want := Node{ID: "ex-old", Alias: "older EX", Address: "127.0.0.1:8443", Role: "foreign"}
	doc, err := json.Marshal(want)
	if err != nil { t.Fatal(err) }
	if _, err := db.Exec(`INSERT INTO nodes (id, doc) VALUES (?, ?)`, want.ID, string(doc)); err != nil { t.Fatal(err) }
	if err := db.Close(); err != nil { t.Fatal(err) }

	store, err := OpenStore(path)
	if err != nil { t.Fatalf("upgrade schema 5 to %d: %v", len(stateMigrations), err) }
	if got := store.st.Nodes[want.ID]; got.ID != want.ID || got.Alias != want.Alias || got.Address != want.Address || got.Role != want.Role {
		t.Fatalf("node changed during migration: got %+v, want %+v", got, want)
	}
	if v, err := stateSchemaVersion(path); err != nil || v != len(stateMigrations) {
		t.Fatalf("upgraded schema version %d, %v", v, err)
	}
	var saved string
	check, err := sql.Open("sqlite", path)
	if err != nil { t.Fatal(err) }
	defer check.Close()
	if err := check.QueryRow(`SELECT doc FROM nodes WHERE id = ?`, want.ID).Scan(&saved); err != nil { t.Fatal(err) }
	if saved != string(doc) { t.Fatalf("migration rewrote existing node: got %q, want %q", saved, doc) }
}

func TestIncidentSnapshotSQLiteRoundTripAndRejectsInvalid(t *testing.T) {
 path:=filepath.Join(t.TempDir(),"state.db")
 var st state
 normalizeState(&st)
 now:=time.Now().UTC().Truncate(time.Second)
 st.IncidentSnapshot.Active["tunnel-1"]=incidentclass.Incident{InstanceID:"tunnel-1",Status:incidentclass.NodeOffline,Reason:"node offline",StartedAt:now,LastObservedAt:now,Observations:1}
 if err:=writeStateDB(path,st);err!=nil {t.Fatal(err)}
 got,err:=readStateDB(path)
 if err!=nil {t.Fatal(err)}
 if !bytes.Equal(canonicalStateJSON(st),canonicalStateJSON(got)) {t.Fatal("incident snapshot lost across SQLite roundtrip")}
 got.IncidentSnapshot.Active["tunnel-2"]=incidentclass.Incident{InstanceID:"mismatched",Status:incidentclass.NodeOffline,StartedAt:now,LastObservedAt:now,Observations:1}
 if err:=writeStateDB(path,got);err==nil {t.Fatal("invalid incident snapshot accepted")}
 after,err:=readStateDB(path)
 if err!=nil {t.Fatal(err)}
 if !bytes.Equal(canonicalStateJSON(st),canonicalStateJSON(after)) {t.Fatal("invalid write replaced valid persisted state")}
}

func TestIncidentSnapshotSQLiteCorruptionFailsClosed(t *testing.T) {
 path:=filepath.Join(t.TempDir(),"state.db")
 var st state
 normalizeState(&st)
 if err:=writeStateDB(path,st);err!=nil {t.Fatal(err)}
 db,err:=openStateDB(path)
 if err!=nil {t.Fatal(err)}
 if _,err:=db.Exec("UPDATE incident_snapshot SET doc = ?", "{bad json");err!=nil {t.Fatal(err)}
 db.Close()
 if _,err:=readStateDB(path);err==nil {t.Fatal("corrupt incident snapshot accepted")}
}

func TestIncidentSnapshotMigrationFromV12PreservesExistingState(t *testing.T) {
 path:=filepath.Join(t.TempDir(),"state.db")
 store:=populatedState(t,path)
 before:=canonicalStateJSON(store.st)
 db,err:=openStateDB(path)
 if err!=nil {t.Fatal(err)}
 if _,err:=db.Exec("DROP TABLE incident_snapshot");err!=nil {t.Fatal(err)}
 if _,err:=db.Exec("DELETE FROM schema_migrations WHERE version = 13");err!=nil {t.Fatal(err)}
 if err:=db.Close();err!=nil {t.Fatal(err)}
 got,err:=OpenStore(path)
 if err!=nil {t.Fatalf("v12 upgrade failed: %v",err)}
 // Schema v12 had no incident snapshot; compare the pre-existing fields only.
 gotLegacy:=got.st
 gotLegacy.IncidentSnapshot=store.st.IncidentSnapshot
 if !bytes.Equal(before,canonicalStateJSON(gotLegacy)) {t.Fatal("v12 migration changed existing BCC state")}
 if len(got.st.IncidentSnapshot.Active)!=0 || len(got.st.IncidentSnapshot.History)!=0 {t.Fatal("v12 migration did not initialize an empty incident snapshot")}
 version,err:=stateSchemaVersion(path)
 if err!=nil || version!=13 {t.Fatalf("migration version %d: %v",version,err)}
}

func TestIncidentSnapshotMissingSingletonFailsClosed(t *testing.T) {
 path:=filepath.Join(t.TempDir(),"state.db")
 var st state
 normalizeState(&st)
 if err:=writeStateDB(path,st);err!=nil {t.Fatal(err)}
 db,err:=openStateDB(path)
 if err!=nil {t.Fatal(err)}
 if _,err:=db.Exec("DELETE FROM incident_snapshot");err!=nil {t.Fatal(err)}
 if err:=db.Close();err!=nil {t.Fatal(err)}
 if _,err:=readStateDB(path);err==nil {t.Fatal("missing incident snapshot row accepted")}
 if _,err:=OpenStore(path);err==nil {t.Fatal("BCC startup accepted missing incident snapshot row")}
}

func TestFreshStoreInitializesIncidentLifecycleMap(t *testing.T) {
 path:=filepath.Join(t.TempDir(),"fresh.db")
 store,err:=OpenStore(path)
 if err!=nil {t.Fatal(err)}
 if store.st.IncidentSnapshot.Active==nil {t.Fatal("fresh store incident map is nil")}
 now:=time.Date(2026,10,15,5,0,0,0,time.UTC)
 store.st.IncidentSnapshot.Active["fresh-tunnel"]=incidentclass.Incident{
  InstanceID:"fresh-tunnel",Status:incidentclass.NodeOffline,
  StartedAt:now,LastObservedAt:now,Observations:1,
 }
 if err:=writeStateDB(path,store.st);err!=nil {t.Fatal(err)}
 reopened,err:=OpenStore(path)
 if err!=nil {t.Fatal(err)}
 if _,ok:=reopened.st.IncidentSnapshot.Active["fresh-tunnel"];!ok {t.Fatal("fresh incident not persisted")}
}


func TestObserveTunnelIncidentScopedDurableAndStaleSafe(t *testing.T) {
 path:=filepath.Join(t.TempDir(),"incident.db")
 store,err:=OpenStore(path)
 if err!=nil {t.Fatal(err)}
 now:=time.Date(2026,10,15,6,0,0,0,time.UTC)
 failed:=incidentclass.Evidence{ObservedAt:now,HeartbeatKnown:true,HeartbeatFresh:false}
 result,err:=store.ObserveTunnelIncident("tunnel-a",now,time.Minute,failed)
 if err!=nil {t.Fatal(err)}
 if result.Status!=incidentclass.NodeOffline {t.Fatalf("unexpected classification: %+v",result)}
 reopened,err:=OpenStore(path)
 if err!=nil {t.Fatal(err)}
 if _,ok:=reopened.st.IncidentSnapshot.Active["tunnel-a"];!ok {t.Fatal("incident not persisted")}
 if len(reopened.st.IncidentSnapshot.Active)!=1 {t.Fatal("unexpected sibling incident")}
 stale:=incidentclass.Evidence{ObservedAt:now.Add(-2*time.Minute),HeartbeatKnown:true,HeartbeatFresh:true,ServiceKnown:true,ServiceRunning:true,HandshakeKnown:true,HandshakeOK:true,DataPathKnown:true,DataPathOK:true}
 result,err=store.ObserveTunnelIncident("tunnel-a",now.Add(time.Second),time.Minute,stale)
 if err!=nil {t.Fatal(err)}
 if result.Status!=incidentclass.Unknown {t.Fatalf("stale evidence was accepted: %+v",result)}
 if len(store.st.IncidentSnapshot.Active)!=1 {t.Fatal("stale evidence closed incident")}
 healthy:=stale
 healthy.ObservedAt=now.Add(2*time.Second)
 result,err=store.ObserveTunnelIncident("tunnel-a",now.Add(2*time.Second),time.Minute,healthy)
 if err!=nil {t.Fatal(err)}
 if result.Status!=incidentclass.Recovered {t.Fatalf("unexpected recovery: %+v",result)}
 reopened,err=OpenStore(path)
 if err!=nil {t.Fatal(err)}
 if len(reopened.st.IncidentSnapshot.Active)!=0 || len(reopened.st.IncidentSnapshot.History)!=1 {t.Fatal("recovery not durably recorded")}
}

func TestObserveTunnelIncidentFailedWriteRestoresMemory(t *testing.T) {
 dir:=t.TempDir()
 path:=filepath.Join(dir,"incident.db")
 store,err:=OpenStore(path)
 if err!=nil {t.Fatal(err)}
 now:=time.Date(2026,10,15,7,0,0,0,time.UTC)
 // An existing directory cannot be opened as a SQLite database. Unlike a\n // missing parent directory, this is a deterministic failure: openStateDB\n // creates missing parent directories automatically.\n store.path=dir
 result,err:=store.ObserveTunnelIncident("rollback-tunnel",now,time.Minute,incidentclass.Evidence{
  ObservedAt:now,HeartbeatKnown:true,HeartbeatFresh:false,
 })
 if err==nil {t.Fatal("expected persistence failure")}
 if result.Status!=incidentclass.NodeOffline {t.Fatalf("unexpected classification: %+v",result)}
 if len(store.st.IncidentSnapshot.Active)!=0 || len(store.st.IncidentSnapshot.History)!=0 {
  t.Fatal("failed persistence published incident in memory")
 }
}
