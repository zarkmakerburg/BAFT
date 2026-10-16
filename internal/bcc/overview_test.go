package bcc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func overviewRig(t *testing.T) (*Server, *Store, time.Time) {
	t.Helper()
	store, err := OpenStore(t.TempDir() + "/s.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []Node{{ID: "ir-1", Alias: "Tehran", Address: "ir.example:22", Role: "worker"},
		{ID: "ex-1", Alias: "Frankfurt", Address: "ex1.example:22", Role: "foreign"},
		{ID: "ex-2", Alias: "Amsterdam", Address: "ex2.example:22", Role: "foreign"}} {
		if _, err := store.UpsertNode(n, ""); err != nil {
			t.Fatal(err)
		}
	}
	app, err := NewServer(store, "admin")
	if err != nil {
		t.Fatal(err)
	}
	return app, store, time.Now().UTC()
}

// report sets a node's last telemetry and a history of cumulative counters
// growing by inStep/outStep bytes per minute.
func report(store *Store, id string, now time.Time, minutes int, inStep, outStep uint64, latency int64) {
	store.mu.Lock()
	defer store.mu.Unlock()
	var h []HistoryPoint
	for i := 0; i <= minutes; i++ {
		h = append(h, HistoryPoint{Timestamp: now.Add(time.Duration(i-minutes) * time.Minute),
			IngressBytes: uint64(i) * inStep, EgressBytes: uint64(i) * outStep, LatencyMS: latency})
	}
	store.st.History[id] = h
	store.st.Telemetry[id] = TelemetryCursor{NodeID: id, LastTelemetry: now}
	n := store.st.Nodes[id]
	n.Health, n.LatencyMS, n.LastChecked = "up", latency, now
	store.st.Nodes[id] = n
}

func TestOverviewCountsOnlyMeasuredValues(t *testing.T) {
	app, _, now := overviewRig(t)
	o := app.Overview(now, 24*time.Hour)
	if o.Servers.Total != 3 || o.Servers.Online != 0 {
		t.Fatalf("servers %+v, want 0 of 3 online", o.Servers)
	}
	if o.LatencyMS != nil || o.Traffic.InBps != nil || o.Traffic.OutBps != nil {
		t.Fatalf("overview invented measurements without telemetry: latency %v traffic %+v", o.LatencyMS, o.Traffic)
	}
	for _, p := range o.Series.Points {
		if p.InBps != nil || p.OutBps != nil {
			t.Fatal("traffic series has a point without telemetry")
		}
	}
	if len(o.Series.Points) < 96 || o.Series.Window != "24h" {
		t.Fatalf("series %s with %d points", o.Series.Window, len(o.Series.Points))
	}
	if o.Version != "dev" {
		t.Fatalf("version %q", o.Version)
	}
}

func TestOverviewAddsExitTrafficOnceAndAveragesLatency(t *testing.T) {
	app, store, now := overviewRig(t)
	// 60 MB per minute in = 8 Mbit/s; IR carries the same bytes and must not add.
	report(store, "ir-1", now, 30, 60_000_000, 30_000_000, 10)
	report(store, "ex-1", now, 30, 60_000_000, 30_000_000, 70)
	report(store, "ex-2", now, 30, 30_000_000, 15_000_000, 90)
	o := app.Overview(now, 24*time.Hour)
	if o.Servers.Online != 3 {
		t.Fatalf("online %d", o.Servers.Online)
	}
	if o.LatencyMS == nil || *o.LatencyMS != (10+70+90)/3 {
		t.Fatalf("average latency %v", o.LatencyMS)
	}
	if o.Traffic.Source != "foreign" || o.Traffic.InBps == nil || *o.Traffic.InBps != 12_000_000 || *o.Traffic.OutBps != 6_000_000 {
		t.Fatalf("traffic %+v in=%v out=%v, want 12 Mbit/s in and 6 out from exits only", o.Traffic, deref(o.Traffic.InBps), deref(o.Traffic.OutBps))
	}
	var last *TrafficPoint
	for i := range o.Series.Points {
		if o.Series.Points[i].InBps != nil {
			last = &o.Series.Points[i]
		}
	}
	if last == nil || *last.InBps != 12_000_000 {
		t.Fatalf("series does not carry the exit traffic: %+v", last)
	}
	for _, n := range o.Nodes {
		if len(n.Spark) == 0 || n.LatencyMS == nil {
			t.Fatalf("node %s lacks latency or spark", n.ID)
		}
	}
	if o.Nodes[0].Role == "foreign" {
		t.Fatal("IR nodes should be listed before exits")
	}
}

func deref(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}

func TestOverviewSkipsCounterResetsAndStaleNodes(t *testing.T) {
	app, store, now := overviewRig(t)
	report(store, "ex-1", now, 10, 6_000_000, 0, 50)
	store.mu.Lock()
	h := store.st.History["ex-1"]
	h[len(h)-1].IngressBytes = 0 // agent restarted: counter went backwards
	store.st.Telemetry["ex-2"] = TelemetryCursor{NodeID: "ex-2", LastTelemetry: now.Add(-time.Hour)}
	store.mu.Unlock()
	o := app.Overview(now, 24*time.Hour)
	for _, p := range o.Series.Points {
		if p.InBps != nil && *p.InBps < 0 {
			t.Fatal("negative rate from a counter reset")
		}
	}
	for _, n := range o.Nodes {
		if n.ID == "ex-2" && (n.Status != "unknown" || n.LatencyMS != nil) {
			t.Fatalf("stale node shown as %s with latency %v", n.Status, n.LatencyMS)
		}
	}
}

func TestNodeLocationPlacesNodeOnTheMap(t *testing.T) {
	app, store, now := overviewRig(t)
	call := func(id, country string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, authReq(http.MethodPost, "/api/nodes/location", "admin", map[string]string{"node_id": id, "country": country}))
		return rr
	}
	if rr := call("ex-1", "de"); rr.Code != 200 {
		t.Fatalf("set location: %d %s", rr.Code, rr.Body)
	}
	if rr := call("ex-1", "XX"); rr.Code != 400 {
		t.Fatalf("unknown country accepted: %d", rr.Code)
	}
	if rr := call("nope", "DE"); rr.Code != 400 {
		t.Fatalf("unknown node accepted: %d", rr.Code)
	}
	// Re-saving the node from the registration form keeps its location.
	if _, err := store.UpsertNode(Node{ID: "ex-1", Alias: "Frankfurt 2", Address: "ex1.example:22", Role: "foreign"}, ""); err != nil {
		t.Fatal(err)
	}
	o := app.Overview(now, 24*time.Hour)
	for _, n := range o.Nodes {
		if n.ID == "ex-1" && (n.Country != "DE" || n.Lat == nil || *n.Lat != 50.11) {
			t.Fatalf("ex-1 not on the map: %+v", n)
		}
		if n.ID == "ex-2" && n.Lat != nil {
			t.Fatal("node without a country got coordinates")
		}
	}
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, authReq(http.MethodGet, "/api/overview?window=7d", "admin", nil))
	var got Overview
	if rr.Code != 200 || json.Unmarshal(rr.Body.Bytes(), &got) != nil || got.Series.Window != "7d" {
		t.Fatalf("overview API: %d %.200s", rr.Code, rr.Body)
	}
	rr = httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, authReq(http.MethodGet, "/api/overview?window=1y", "admin", nil))
	if rr.Code != 400 {
		t.Fatalf("bad window accepted: %d", rr.Code)
	}
	rr = httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/overview", nil))
	if rr.Code != 401 {
		t.Fatalf("overview without auth: %d", rr.Code)
	}
}

func TestUptimeCountsOnlyCoveredTime(t *testing.T) {
	now := time.Now().UTC()
	at := func(m int) HistoryPoint { return HistoryPoint{Timestamp: now.Add(time.Duration(m) * time.Minute)} }
	if uptime(nil, now, 24*time.Hour, 3*time.Minute) != nil {
		t.Fatal("uptime without reports must be unknown")
	}
	// Reports every minute for an hour, then a 30-minute hole, then another 30 minutes.
	var h []HistoryPoint
	for m := -120; m <= -60; m++ {
		h = append(h, at(m))
	}
	for m := -30; m <= 0; m++ {
		h = append(h, at(m))
	}
	got := *uptime(h, now, 24*time.Hour, 3*time.Minute)
	if want := 90.0 / 120 * 100; got < want-0.5 || got > want+0.5 {
		t.Fatalf("uptime %.1f%%, want %.1f%%", got, want)
	}
}

func TestOverviewHealthAndFinanceAreHonest(t *testing.T) {
	app, store, now := overviewRig(t)
	report(store, "ex-1", now, 30, 1e6, 2e6, 40)
	o := app.Overview(now, 24*time.Hour)
	if o.Health.BackupsConfigured || o.Health.LastBackup != nil {
		t.Fatal("backups reported without a backup directory")
	}
	if o.Health.AgentsStale != 3 {
		t.Fatalf("agents stale %d, want 3 (no agent has called BCC)", o.Health.AgentsStale)
	}
	if len(o.Finance.Rows) != 0 || o.Finance.Month == "" {
		t.Fatalf("finance %+v, want the month with no rows", o.Finance)
	}
	for _, n := range o.Nodes {
		if n.ID == "ex-1" && (n.InBps == nil || n.UptimePct == nil) {
			t.Fatal("reporting node lost its rate or uptime")
		}
		if n.ID == "ex-2" && (n.InBps != nil || n.UptimePct != nil) {
			t.Fatal("silent node has a rate or uptime")
		}
	}
}
