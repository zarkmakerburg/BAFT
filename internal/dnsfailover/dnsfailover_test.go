package dnsfailover

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeCF is an in-memory Cloudflare DNS records API.
type fakeCF struct {
	mu      sync.Mutex
	recs    map[string]dnsRecord
	next    int
	calls   []string
	failAll bool
}

func (f *fakeCF) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.Method)
	if r.Header.Get("Authorization") != "Bearer tok" || f.failAll {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":10000}]}`))
		return
	}
	write := func(v any) {
		b, _ := json.Marshal(v)
		_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":` + string(b) + `}`))
	}
	switch r.Method {
	case http.MethodGet:
		if strings.HasSuffix(r.URL.Path, "/zones") {
			if r.URL.Query().Get("name") == "example.com" {
				write([]map[string]string{{"id": "z1", "name": "example.com"}})
			} else {
				write([]map[string]string{})
			}
			return
		}
		if !strings.Contains(r.URL.Path, "/zones/z1/") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":7003}]}`))
			return
		}
		out := []dnsRecord{}
		for _, rec := range f.recs {
			if rec.Name == r.URL.Query().Get("name") {
				out = append(out, rec)
			}
		}
		write(out)
	case http.MethodPost:
		var rec dnsRecord
		_ = json.NewDecoder(r.Body).Decode(&rec)
		f.next++
		rec.ID = "id" + string(rune('a'+f.next))
		f.recs[rec.ID] = rec
		write(rec)
	case http.MethodDelete:
		id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		delete(f.recs, id)
		write(map[string]string{"id": id})
	}
}

func (f *fakeCF) contents() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, r := range f.recs {
		out = append(out, r.Content)
	}
	sortStrings(out)
	return out
}

func sortStrings(s []string) {
	for i := range s {
		for j := i + 1; j < len(s); j++ {
			if s[j] < s[i] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}

func testConfig() Config {
	c := Config{ZoneID: "z1", Record: "t.example.com", TokenFile: "/x", Targets: []Target{
		{Name: "a", Address: "192.0.2.1", Probe: "192.0.2.1:443"},
		{Name: "b", Address: "192.0.2.2", Probe: "192.0.2.2:443"},
		{Name: "c", Address: "192.0.2.3", Probe: "192.0.2.3:443"},
	}}
	if err := c.normalize(); err != nil {
		panic(err)
	}
	return c
}

func rig(t *testing.T, down map[string]bool) (*Controller, *fakeCF) {
	t.Helper()
	f := &fakeCF{recs: map[string]dnsRecord{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	probe := func(_ context.Context, tg Target, _ time.Duration) bool { return !down[tg.Name] }
	c := New(testConfig(), "tok", probe, t.Logf)
	c.apiBase = srv.URL
	return c, f
}

func TestPublishesAllHealthyTargets(t *testing.T) {
	c, f := rig(t, map[string]bool{})
	c.ProbeOnce(context.Background())
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := f.contents(); !reflect.DeepEqual(got, []string{"192.0.2.1", "192.0.2.2", "192.0.2.3"}) {
		t.Fatalf("records = %v", got)
	}
	for _, r := range f.recs {
		if r.Comment != ManagedComment || r.TTL != 60 || r.Proxied || r.Type != "A" {
			t.Fatalf("record not managed DNS-only A with ttl 60: %+v", r)
		}
	}
}

func TestDownTargetWithdrawnOnlyAfterFailAfterAndRestoredAfterRecoverAfter(t *testing.T) {
	down := map[string]bool{}
	c, f := rig(t, down)
	ctx := context.Background()
	c.ProbeOnce(ctx)
	_, _ = c.Reconcile(ctx)

	down["b"] = true
	for i := 0; i < 2; i++ { // below fail_after=3
		c.ProbeOnce(ctx)
		_, _ = c.Reconcile(ctx)
	}
	if len(f.contents()) != 3 {
		t.Fatalf("withdrawn too early: %v", f.contents())
	}
	c.ProbeOnce(ctx)
	_, _ = c.Reconcile(ctx)
	if got := f.contents(); !reflect.DeepEqual(got, []string{"192.0.2.1", "192.0.2.3"}) {
		t.Fatalf("after 3 failures records = %v", got)
	}

	down["b"] = false
	c.ProbeOnce(ctx)
	c.ProbeOnce(ctx)
	_, _ = c.Reconcile(ctx)
	if len(f.contents()) != 2 {
		t.Fatalf("restored too early: %v", f.contents())
	}
	c.ProbeOnce(ctx)
	_, _ = c.Reconcile(ctx)
	if len(f.contents()) != 3 {
		t.Fatalf("not restored after recover_after: %v", f.contents())
	}
}

func TestFlappingProbeNeverMovesTraffic(t *testing.T) {
	down := map[string]bool{}
	c, f := rig(t, down)
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		down["a"] = i%3 == 0 // never 3 in a row
		c.ProbeOnce(ctx)
		_, _ = c.Reconcile(ctx)
	}
	if len(f.contents()) != 3 {
		t.Fatalf("flapping target withdrawn: %v", f.contents())
	}
}

func TestAllDownLeavesRecordsUnchanged(t *testing.T) {
	down := map[string]bool{}
	c, f := rig(t, down)
	ctx := context.Background()
	c.ProbeOnce(ctx)
	_, _ = c.Reconcile(ctx)
	down["a"], down["b"], down["c"] = true, true, true
	for i := 0; i < 5; i++ {
		c.ProbeOnce(ctx)
		if _, err := c.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if len(f.contents()) != 3 {
		t.Fatalf("all-down withdrew records: %v", f.contents())
	}
}

func TestForeignRecordStopsReconcile(t *testing.T) {
	c, f := rig(t, map[string]bool{})
	f.recs["x"] = dnsRecord{ID: "x", Type: "A", Name: "t.example.com", Content: "198.51.100.9", TTL: 300}
	c.ProbeOnce(context.Background())
	if _, err := c.Reconcile(context.Background()); err == nil || !strings.Contains(err.Error(), "not created by baft") {
		t.Fatalf("err = %v", err)
	}
	if len(f.recs) != 1 {
		t.Fatalf("foreign zone touched: %v", f.recs)
	}
	for _, m := range f.calls {
		if m != http.MethodGet {
			t.Fatalf("wrote despite foreign record: %v", f.calls)
		}
	}
}

func TestAddsBeforeRemoving(t *testing.T) {
	down := map[string]bool{"c": true}
	c, f := rig(t, down)
	ctx := context.Background()
	// start with only c published (e.g. left from an earlier run)
	f.recs["old"] = dnsRecord{ID: "old", Type: "A", Name: "t.example.com", Content: "192.0.2.3", TTL: 60, Comment: ManagedComment}
	for i := 0; i < 3; i++ {
		c.ProbeOnce(ctx)
	}
	f.calls = nil
	if _, err := c.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.calls, ","); got != "GET,POST,POST,DELETE" {
		t.Fatalf("call order = %s", got)
	}
}

func TestAPIErrorIsReturnedAndNothingAssumed(t *testing.T) {
	c, f := rig(t, map[string]bool{})
	f.failAll = true
	if _, err := c.Reconcile(context.Background()); err == nil || !strings.Contains(err.Error(), "10000") {
		t.Fatalf("err = %v", err)
	}
}

func TestConfigValidation(t *testing.T) {
	dir := t.TempDir()
	write := func(s string) string {
		p := filepath.Join(dir, "c.json")
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	bad := []string{
		`{"zone_id":"z","record":"t.example.com","token_file":"f","targets":[{"name":"a","address":"1.2.3.4","probe":"1.2.3.4:1"}]}`,
		`{"zone_id":"z","record":"t.example.com","token_file":"f","targets":[{"name":"a","address":"host","probe":"1.2.3.4:1"},{"name":"b","address":"1.2.3.5","probe":"1.2.3.5:1"}]}`,
		`{"zone_id":"z","record":"t.example.com","token_file":"f","targets":[{"name":"a","address":"1.2.3.4","probe":"1.2.3.4:1"},{"name":"b","address":"1.2.3.4","probe":"1.2.3.4:2"}]}`,
		`{"zone_id":"z","record":"t.example.com","token_file":"f","ttl":5,"targets":[{"name":"a","address":"1.2.3.4","probe":"1.2.3.4:1"},{"name":"b","address":"1.2.3.5","probe":"1.2.3.5:1"}]}`,
		`{"zone_id":"z","record":"t.example.com","token_file":"f","unknown":1,"targets":[]}`,
	}
	for i, s := range bad {
		if _, err := LoadConfig(write(s)); err == nil {
			t.Fatalf("case %d accepted", i)
		}
	}
	good := `{"zone_id":"z","record":"t.example.com","token_file":"f","targets":[{"name":"a","address":"1.2.3.4","probe":"1.2.3.4:1"},{"name":"b","address":"2001:db8::1","probe":"[2001:db8::1]:1"}]}`
	c, err := LoadConfig(write(good))
	if err != nil || c.TTL != 60 || c.FailAfter != 3 {
		t.Fatalf("good config: %+v %v", c, err)
	}
	if recordType("2001:db8::1") != "AAAA" {
		t.Fatal("ipv6 must be AAAA")
	}
}

func TestReadTokenRefusesLoosePermissions(t *testing.T) {
	p := filepath.Join(t.TempDir(), "tok")
	if err := os.WriteFile(p, []byte("abc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadToken(p); err == nil {
		t.Fatal("world-readable token accepted")
	}
	if err := os.Chmod(p, 0o600); err != nil {
		t.Fatal(err)
	}
	if tok, err := ReadToken(p); err != nil || tok != "abc" {
		t.Fatalf("tok=%q err=%v", tok, err)
	}
}

func TestZoneIsFoundFromTheRecordName(t *testing.T) {
	c, f := rig(t, map[string]bool{})
	c.cfg.ZoneID = ""
	c.cfg.Record = "t.sub.example.com"
	id, err := c.FindZone(context.Background())
	if err != nil || id != "z1" {
		t.Fatalf("zone=%q err=%v", id, err)
	}
	c.cfg.Record = "t.example.com"
	c.ProbeOnce(context.Background())
	if _, err := c.Reconcile(context.Background()); err != nil || len(f.contents()) != 3 {
		t.Fatalf("reconcile with discovered zone: %v %v", err, f.contents())
	}
	c.cfg.ZoneID = ""
	c.cfg.Record = "t.other.org"
	if _, err := c.FindZone(context.Background()); err == nil {
		t.Fatal("found a zone the token cannot see")
	}
}
