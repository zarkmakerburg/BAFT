//go:build unix

package agent

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
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
	"syscall"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/bcc"
	"github.com/zarkmakerburg/baft/internal/tunnelnode"
)

// sharedServices makes supervision explicit. The default controller starts
// actual baft processes; the operational LAB uses node.Runtime in this process
// because its installed-policy inventory requires a sole process in the netns.
type sharedServices interface {
	tunnelnode.System
	identity(string) any
	close()
}

type sharedRealOptions struct {
	ctx       context.Context
	services  func(*flowNode) sharedServices
	configure func(*flow, *httptest.Server)
	request   func(*bcc.TunnelRequest, func() int)
	observe   func(string, *flow, *httptest.Server, bcc.Tunnel, bcc.Tunnel)
}

// processServices replaces only systemd's process supervision. Pairing,
// signed jobs, HTTPS ACKs, reloads and all payload bytes use the real product.
type processServices struct {
	mu        sync.Mutex
	units     string
	processes map[string]*serviceProcess
	enabled   map[string]bool
}

type serviceProcess struct {
	cmd  *exec.Cmd
	done chan struct{}
}

func (s *processServices) stop(name string) {
	p := s.processes[name]
	if p == nil {
		return
	}
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-p.done:
	case <-time.After(3 * time.Second):
		_ = p.cmd.Process.Kill()
		<-p.done
	}
	delete(s.processes, name)
}

func (s *processServices) Systemctl(_ context.Context, args ...string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(args) == 0 {
		return "", fmt.Errorf("missing service action")
	}
	if args[0] == "daemon-reload" {
		return "", nil
	}
	if len(args) < 2 {
		return "", fmt.Errorf("missing service name")
	}
	name := strings.TrimSuffix(args[1], ".service")
	alive := func() bool {
		p := s.processes[name]
		if p == nil {
			return false
		}
		select {
		case <-p.done:
			return false
		default:
			return true
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
		if alive() {
			return "active", nil
		}
		return "inactive", nil
	case "show":
		if alive() {
			return "0", nil
		}
		return "1", nil
	case "stop":
		s.stop(name)
	case "reload":
		if !alive() {
			return "", fmt.Errorf("cannot reload stopped service %s", name)
		}
		return "", s.processes[name].cmd.Process.Signal(syscall.SIGHUP)
	case "restart", "start":
		s.stop(name)
		raw, err := os.ReadFile(filepath.Join(s.units, name+".service"))
		if err != nil {
			return "", err
		}
		var command []string
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(line, "ExecStart=") {
				command = strings.Fields(strings.TrimPrefix(line, "ExecStart="))
				break
			}
		}
		if len(command) == 0 {
			return "", fmt.Errorf("no ExecStart for %s", name)
		}
		cmd := exec.Command(command[0], command[1:]...)
		log, err := os.Create(filepath.Join(s.units, name+".log"))
		if err != nil {
			return "", err
		}
		cmd.Stdout, cmd.Stderr = log, log
		if err := cmd.Start(); err != nil {
			log.Close()
			return "", err
		}
		p := &serviceProcess{cmd: cmd, done: make(chan struct{})}
		s.processes[name] = p
		go func() { _ = cmd.Wait(); log.Close(); close(p.done) }()
	default:
		return "", fmt.Errorf("unsupported service action %s", args[0])
	}
	return "", nil
}

func (s *processServices) Run(ctx context.Context, binary string, args ...string) (string, error) {
	raw, err := exec.CommandContext(ctx, binary, args...).CombinedOutput()
	return strings.TrimSpace(string(raw)), err
}

func (s *processServices) pid(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p := s.processes[name]; p != nil {
		return p.cmd.Process.Pid
	}
	return 0
}

func (s *processServices) identity(name string) any {
	if pid := s.pid(name); pid != 0 {
		return pid
	}
	return nil
}

func (s *processServices) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for name := range s.processes {
		s.stop(name)
	}
}

func markedTarget(t *testing.T, marker string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				scan := bufio.NewScanner(c)
				for scan.Scan() {
					if _, err := fmt.Fprintf(c, "%s:%s\n", marker, scan.Text()); err != nil {
						return
					}
				}
			}()
		}
	}()
	return ln.Addr().String()
}

func realRoundTrip(c net.Conn, marker string) error {
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.WriteString(c, "payload\n"); err != nil {
		return err
	}
	got, err := bufio.NewReader(c).ReadString('\n')
	if err != nil {
		return err
	}
	if got != marker+":payload\n" {
		return fmt.Errorf("wrong tenant payload %q", got)
	}
	return nil
}

func readyFlow(t *testing.T, address, marker string) net.Conn {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", address, time.Second)
		if err == nil {
			err = realRoundTrip(c, marker)
			if err == nil {
				return c
			}
			c.Close()
		}
		last = err
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("authenticated %s flow not ready: %v", marker, last)
	return nil
}

func snapshotRegular(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, e os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			out[strings.TrimPrefix(path, root)] = string(raw)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestBCCSignedSharedSameIRRealFlowsPreserveOwner(t *testing.T) {
	runBCCSignedSharedSameIRRealFlows(t, sharedRealOptions{})
}

func runBCCSignedSharedSameIRRealFlows(t *testing.T, opts sharedRealOptions) {
	t.Helper()
	ctx := opts.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	dir := t.TempDir()
	store, err := bcc.OpenStore(filepath.Join(dir, "bcc", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	app, err := bcc.NewServer(store, "admin")
	if err != nil {
		t.Fatal(err)
	}
	key, _, err := bcc.LoadOrCreateJobKey(filepath.Join(dir, "bcc", "job-key"))
	if err != nil {
		t.Fatal(err)
	}
	app.ConfigureJobSigning(key)
	f := &flow{t: t, app: app, store: store, h: app.Handler(), dir: dir, jobKey: key}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { f.h.ServeHTTP(w, r) }))
	t.Cleanup(srv.Close)
	used := map[int]bool{}
	port := func() int {
		for {
			n := freeFlowPort(t)
			if !used[n] {
				used[n] = true
				return n
			}
		}
	}
	addr := func() string { return fmt.Sprintf("127.0.0.1:%d", port()) }
	f.exPort = port()
	f.irRoute = addr()
	f.ex = f.addNode(t, srv.URL, key.Public().(ed25519.PublicKey), "ex-1", "foreign", "127.0.0.1:1")
	f.ir = f.addNode(t, srv.URL, key.Public().(ed25519.PublicKey), "ir-1", "worker", "127.0.0.1:2")
	controllers := map[string]sharedServices{}
	for index, n := range []*flowNode{f.ex, f.ir} {
		var s sharedServices = &processServices{units: n.tn.UnitDir, processes: map[string]*serviceProcess{}, enabled: map[string]bool{}}
		if opts.services != nil {
			s = opts.services(n)
		}
		controllers[n.id] = s
		n.tn.System = s
		n.tn.Settle = 100 * time.Millisecond
		n.agent.cfg.System = s
		// Each enrolled host has its own source address. Sharing one source
		// IP would combine both hosts' rapid test polls into one rate budget.
		transport := srv.Client().Transport.(*http.Transport).Clone()
		dialer := &net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP(fmt.Sprintf("127.0.0.%d", index+2))}}
		transport.DialContext = dialer.DialContext
		n.agent.cfg.HTTP = &http.Client{Transport: transport}
		t.Cleanup(transport.CloseIdleConnections)
		t.Cleanup(s.close)
	}
	if opts.configure != nil {
		opts.configure(f, srv)
	}
	call := func(method, path string, input, output any) {
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
		if resp.StatusCode != 200 && resp.StatusCode != 202 {
			raw, _ := io.ReadAll(resp.Body)
			t.Fatalf("%s %s: %d %s", method, path, resp.StatusCode, raw)
		}
		if output != nil {
			if err := json.NewDecoder(resp.Body).Decode(output); err != nil {
				t.Fatal(err)
			}
		}
	}
	create := func(req bcc.TunnelRequest) bcc.Tunnel {
		t.Helper()
		if opts.request != nil {
			opts.request(&req, port)
		}
		for _, n := range []*flowNode{f.ex, f.ir} {
			if _, err := n.agent.RunOnce(ctx); err != nil {
				t.Fatal(err)
			}
		}
		var plan bcc.Plan
		call("POST", "/api/tunnels/plan", req, &plan)
		if !plan.OK {
			t.Fatalf("plan refused: %+v", plan)
		}
		req = plan.Request
		req.PlanHash = plan.Hash
		var tn bcc.Tunnel
		call("POST", "/api/tunnels", req, &tn)
		tn = runSharedFlow(ctx, f, tn.ID)
		if tn.Phase != bcc.TunnelActive {
			t.Fatalf("real activation: %s %s", tn.Phase, tn.Error)
		}
		return tn
	}
	req := f.plan()
	req.Engine = "baft"
	req.InstanceID = "shared-real"
	req.Target = markedTarget(t, "owner")
	req.EXMetricsListen = addr()
	req.IRMetricsListen = addr()
	owner := create(req)
	observe := func(event string, joined bcc.Tunnel) {
		if opts.observe != nil {
			opts.observe(event, f, srv, owner, joined)
		}
	}
	observe("owner", bcc.Tunnel{})
	ownerIR, err := f.ir.tn.ForInstance(owner.InstanceID, owner.IRMetricsListen)
	if err != nil {
		t.Fatal(err)
	}
	ownerConfig := snapshotRegular(t, ownerIR.ConfigDir)
	ownerState := snapshotRegular(t, ownerIR.StateDir)
	ownerUnit, err := os.ReadFile(filepath.Join(ownerIR.UnitDir, ownerIR.Service+".service"))
	if err != nil {
		t.Fatal(err)
	}
	exService := "baft-" + owner.InstanceID
	exIdentity := controllers[f.ex.id].identity(exService)
	conn := readyFlow(t, owner.RouteListen, "owner")
	defer conn.Close()
	joined := create(bcc.TunnelRequest{Engine: "baft", JoinExistingTunnelID: owner.ID, EXNode: owner.EXNode, IRNode: owner.IRNode, Target: markedTarget(t, "tenant"), IRMetricsListen: addr()})
	if joined.IRInstanceID == "" || joined.IRInstanceID == owner.InstanceID {
		t.Fatal("same-IR service was not isolated")
	}
	tenantConn := readyFlow(t, joined.RouteListen, "tenant")
	tenantConn.Close()
	if err := realRoundTrip(conn, "owner"); err != nil {
		t.Fatalf("join interrupted persistent owner: %v", err)
	}
	observe("join", joined)
	var replacement bcc.Plan
	replacementBody, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	replacementRequest, err := http.NewRequest("POST", srv.URL+"/api/tunnels/plan", bytes.NewReader(replacementBody))
	if err != nil {
		t.Fatal(err)
	}
	replacementRequest.Header.Set("Authorization", "Bearer admin")
	replacementResponse, err := srv.Client().Do(replacementRequest)
	if err != nil {
		t.Fatal(err)
	}
	refusal, err := io.ReadAll(replacementResponse.Body)
	replacementResponse.Body.Close()
	if err != nil || replacementResponse.StatusCode != http.StatusBadRequest || !strings.Contains(string(refusal), "retire its tenants") {
		t.Fatalf("physical owner replacement was not refused for its active tenant: %d %s", replacementResponse.StatusCode, refusal)
	}
	tenantIR, err := f.ir.tn.ForInstance(joined.IRInstanceID, joined.IRMetricsListen)
	if err != nil {
		t.Fatal(err)
	}
	tenantIdentity := controllers[f.ir.id].identity(tenantIR.Service)
	if _, err := controllers[f.ir.id].Systemctl(ctx, "restart", tenantIR.Service); err != nil {
		t.Fatal(err)
	}
	tenantConn = readyFlow(t, joined.RouteListen, "tenant")
	tenantConn.Close()
	if current := controllers[f.ir.id].identity(tenantIR.Service); tenantIdentity == nil || current == nil || current == tenantIdentity {
		t.Fatal("tenant IR service identity did not change across its restart")
	}
	if err := realRoundTrip(conn, "owner"); err != nil {
		t.Fatalf("tenant restart interrupted owner: %v", err)
	}
	observe("restart", joined)
	var retirement bcc.DecommissionPlan
	call("POST", "/api/tunnels/decommission/plan", map[string]string{"id": joined.ID}, &retirement)
	if !retirement.OK {
		t.Fatalf("tenant retirement refused: %+v", retirement)
	}
	call("POST", "/api/tunnels/decommission", map[string]string{"id": joined.ID, "plan_hash": retirement.Hash}, nil)
	for i := 0; i < 40; i++ {
		for _, n := range []*flowNode{f.ex, f.ir} {
			if _, err := n.agent.RunOnce(ctx); err != nil {
				t.Fatal(err)
			}
		}
		if f.tunnel(joined.ID).Phase == bcc.TunnelDecommissioned {
			break
		}
	}
	if tn := f.tunnel(joined.ID); tn.Phase != bcc.TunnelDecommissioned {
		t.Fatalf("tenant did not retire: %s: %s", tn.Phase, tn.Error)
	}
	if err := realRoundTrip(conn, "owner"); err != nil {
		t.Fatalf("tenant retire interrupted owner: %v", err)
	}
	observe("retire", joined)
	if exIdentity == nil || controllers[f.ex.id].identity(exService) != exIdentity {
		t.Fatal("shared EX service identity changed")
	}
	if !reflect.DeepEqual(ownerConfig, snapshotRegular(t, ownerIR.ConfigDir)) || !reflect.DeepEqual(ownerState, snapshotRegular(t, ownerIR.StateDir)) {
		t.Fatal("tenant lifecycle changed owner config/key/state")
	}
	afterUnit, err := os.ReadFile(filepath.Join(ownerIR.UnitDir, ownerIR.Service+".service"))
	if err != nil || !bytes.Equal(ownerUnit, afterUnit) {
		t.Fatal("tenant lifecycle changed owner unit")
	}
	if tn := f.tunnel(owner.ID); tn.Phase != bcc.TunnelActive {
		t.Fatalf("owner BCC card is not active: %s", tn.Phase)
	}
	call("POST", "/api/tunnels/plan", req, &replacement)
	if !replacement.OK {
		t.Fatal("physical owner remained locked after its tenant was retired")
	}
	f.noSecretsInBCC()
}

// runSharedFlow is the signed-job activation loop with the scenario's lifetime
// context, so operational listeners created during activation are cleaned up.
func runSharedFlow(ctx context.Context, f *flow, id string) bcc.Tunnel {
	f.t.Helper()
	for i := 0; i < 40; i++ {
		for _, n := range []*flowNode{f.ex, f.ir} {
			if _, err := n.agent.RunOnce(ctx); err != nil {
				f.t.Fatalf("%s RunOnce: %v", n.id, err)
			}
		}
		switch tn := f.tunnel(id); tn.Phase {
		case bcc.TunnelActive, bcc.TunnelRolledBack, bcc.TunnelRollbackFailed:
			return tn
		}
	}
	f.t.Fatalf("tunnel %s did not settle: %+v", id, f.tunnel(id))
	return bcc.Tunnel{}
}
