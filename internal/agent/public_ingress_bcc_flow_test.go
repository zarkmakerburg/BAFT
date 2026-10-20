package agent

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/bcc"
	"github.com/zarkmakerburg/baft/internal/release"
)

// This exercises real BCC enqueue, durable job storage, agentAction signing,
// authenticated pull, agent execution, ACK and durable evidence retrieval.
func TestPublicIngressPreflightRealBCCAgentRoundTrip(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires Linux procfs")
	}
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	// The public port and loopback destination are independent resources.
	// Keep both listeners bound to exercise real socket observations without
	// creating a forwarding loop that production planning must reject.
	target, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	store, err := bcc.OpenStore(filepath.Join(t.TempDir(), "bcc.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []bcc.Node{{ID: "ir", Role: "worker", Address: "127.0.0.1:22"}, {ID: "ex", Role: "foreign", Address: "192.0.2.20:22"}} {
		if _, err := store.UpsertNode(n, "token-"+n.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.PullJobs("ir", "token-ir"); err != nil {
		t.Fatal(err)
	}
	tunnel, err := store.CreateTunnel(bcc.TunnelRequest{IRNode: "ir", EXNode: "ex", RouteListen: ln.Addr().String(), Port: 14443, Target: "127.0.0.1:2443"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	app, err := bcc.NewServer(store, "admin")
	if err != nil {
		t.Fatal(err)
	}
	pub, key, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	app.ConfigureJobSigning(key)
	srv := httptest.NewServer(app.Handler())
	defer srv.Close()
	request := bcc.PublicIngressPreflightRequest{IRNode: "ir", TunnelID: tunnel.ID, PublicPort: ln.Addr().(*net.TCPAddr).Port, LocalTarget: target.Addr().String()}
	body, _ := json.Marshal(request)
	req := httptest.NewRequest(http.MethodPost, "/api/ingress/preflight", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer admin")
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, req)
	if rr.Code != 202 {
		t.Fatalf("enqueue=%d %s", rr.Code, rr.Body.String())
	}
	var response struct {
		Job   bcc.Job `json:"job"`
		Ready bool    `json:"ready_to_apply"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	sys := &fakeSystem{}
	a, err := New(Config{BCCURL: srv.URL, NodeID: "ir", Token: "token-ir", BCCJobKey: pub, StateDir: t.TempDir(), System: sys})
	if err != nil {
		t.Fatal(err)
	}
	results, err := a.RunOnce(context.Background())
	if err != nil || len(results) != 1 || results[0].Status != "succeeded" || results[0].Action != agentjob.ActionPublicIngressPreflight {
		t.Fatalf("real BCC dispatch: %+v %v", results, err)
	}
	reopened, err := bcc.OpenStore(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	var receipt bcc.Job
	for _, job := range reopened.ListJobs() {
		if job.ID == response.Job.ID {
			receipt = job
		}
	}
	var ev publicIngressPreflightEvidence
	if err := json.Unmarshal([]byte(receipt.Output), &ev); err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "succeeded" || ev.PublicPortState != "occupied" || !ev.TargetListenerObserved || ev.ReadyToApply || response.Ready || len(sys.calls) > 0 {
		t.Fatalf("invalid roundtrip receipt: %+v %+v", receipt, ev)
	}
	after, _ := reopened.GetTunnel(tunnel.ID)
	if after.Phase != tunnel.Phase {
		t.Fatalf("preflight modified tunnel lifecycle: %s -> %s", tunnel.Phase, after.Phase)
	}
	readReq := httptest.NewRequest(http.MethodGet, "/api/ingress/preflight?id="+receipt.ID, nil)
	readReq.Header.Set("Authorization", "Bearer admin")
	readResult := httptest.NewRecorder()
	app.Handler().ServeHTTP(readResult, readReq)
	var exposed bcc.PublicIngressPreflightReceipt
	if err := json.Unmarshal(readResult.Body.Bytes(), &exposed); err != nil {
		t.Fatal(err)
	}
	if readResult.Code != http.StatusOK || exposed.Status != "succeeded" || len(exposed.Evidence) == 0 || !exposed.ReadOnly || exposed.ReadyToApply {
		t.Fatalf("operator receipt lookup: %d %s", readResult.Code, readResult.Body.String())
	}
}
