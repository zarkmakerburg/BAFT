//go:build linux

package bcc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agent"
	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressinventory"
	"github.com/zarkmakerburg/baft/internal/release"
)

// This is an ingress lifecycle LAB with a persisted active-tunnel prerequisite,
// not a two-node tunnel/transport or public WAN acceptance. All public ingress
// effects, signed polling, TCP bytes and authenticated receipts are real.

func readOperationalIngressStatus(t *testing.T, endpoint *httptest.Server, client *http.Client, tunnelID, adminToken string) publicIngressOperationResponse {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, endpoint.URL+"/api/ingress/deploy?tunnel_id="+tunnelID, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("ingress status GET=%d body=%s", resp.StatusCode, body)
	}
	var status publicIngressOperationResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func TestOperationalIngressFullLifecycleLAB(t *testing.T) {
	requireOperationalLifecycleLAB(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	target := operationalEchoListener(t)
	foreign := operationalEchoListener(t)
	s, spec := reservationStore(t)
	spec.LocalTarget = target.Addr().String()
	probe, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, portText, err := net.SplitHostPort(probe.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if err := probe.Close(); err != nil {
		t.Fatal(err)
	}
	spec.PublicPort, err = strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	const token = "gamma-real-operational-enrollment"
	pub, key, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	tun := s.st.Tunnels[spec.TunnelID]
	tun.RouteListen, tun.ClientPort = spec.LocalTarget, spec.PublicPort
	// The active tunnel is a prerequisite fixture. The actual ingress review
	// below binds one immutable plan digest to the physical target and port.
	review, err := json.Marshal(spec)
	if err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	digest := sha256.Sum256(review)
	tun.PlanHash = hex.EncodeToString(digest[:])
	s.st.Tunnels[tun.ID] = tun
	node := s.st.Nodes[spec.IRNode]
	node.AgentTokenHash, node.AgentSeen = tokenHash(token), time.Now().UTC()
	s.st.Nodes[node.ID] = node
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()
	app, err := NewServer(s, "gamma-admin")
	if err != nil {
		t.Fatal(err)
	}
	app.ConfigureJobSigning(key)
	endpoint := httptest.NewTLSServer(app.Handler())
	defer endpoint.Close()
	client := endpoint.Client()
	client.Timeout = 5 * time.Second
	response, err := client.Get(endpoint.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.TLS == nil || len(response.TLS.VerifiedChains) == 0 {
		t.Fatal("BCC TLS certificate was not validated")
	}
	deployResponse := postIngressOperation(t, endpoint, "deploy", spec, tun.PlanHash, "gamma-admin")
	if deployResponse.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(deployResponse.Body)
		deployResponse.Body.Close()
		t.Fatalf("public BCC deploy API status=%d body=%s", deployResponse.StatusCode, body)
	}
	var deployResult publicIngressOperationResponse
	if err := json.NewDecoder(deployResponse.Body).Decode(&deployResult); err != nil {
		deployResponse.Body.Close()
		t.Fatal(err)
	}
	deployResponse.Body.Close()
	op, ok := s.GetPublicIngressOperation(spec.TunnelID)
	if !ok || op.Command == nil || op.Dispatch == nil || op.Dispatch.Phase != "queued" || op.Command.JobID != deployResult.JobID || deployResult.ReadyToApply || !deployResult.ReconciliationPending {
		t.Fatalf("public deploy API did not queue exact reviewed command fail-closed: result=%+v operation=%+v", deployResult, op)
	}
	command := *op.Command
	configureOperationalLifecycleLAB(t, spec.IRNode, pub, foreign.Addr().String())
	baseline := operationalNFT(t, "-j", "-a", "list", "table", "inet", "gamma_unrelated")
	a, err := agent.New(agent.Config{BCCURL: endpoint.URL, NodeID: spec.IRNode, Token: token, BCCJobKey: pub, StateDir: t.TempDir(), HTTP: client})
	if err != nil {
		t.Fatal(err)
	}
	checkRun := func(jobID, phase string) {
		t.Helper()
		results, err := a.RunOnce(ctx)
		if err != nil || len(results) != 1 || results[0].JobID != jobID || results[0].Status != "succeeded" || !strings.Contains(results[0].Detail, "operational ingress "+phase+" with authenticated receipt") || strings.Contains(results[0].Detail, "ack failed") {
			t.Fatalf("normal signed RunOnce %s: %+v %v", phase, results, err)
		}
	}
	checkRun(command.JobID, "applied")
	applyStatus := readOperationalIngressStatus(t, endpoint, client, spec.TunnelID, "gamma-admin")
	if applyStatus.Action != agentjob.ActionPublicIngressApply || applyStatus.ReceiptPhase != "applied" || !applyStatus.ReceiptVerified || applyStatus.ReadyToApply || applyStatus.PlanHash != tun.PlanHash || applyStatus.PublicPort != spec.PublicPort {
		t.Fatalf("status API did not expose exact verified Apply receipt safely: %+v", applyStatus)
	}
	applied, ok := s.GetPublicIngressOperation(spec.TunnelID)
	if !ok || applied.Receipt == nil || applied.Receipt.ValidateForCommand(command) != nil || applied.Receipt.Phase != "applied" || !applied.Receipt.OwnershipVerified || applied.Status != PublicIngressPending {
		t.Fatalf("actual receipt not durably pending: %+v", applied)
	}
	if applied.Receipt.Binding.ConfigPort != uint16(spec.PublicPort) || applied.Receipt.ListenerEvidence.SocketIdentity == "" {
		t.Fatal("selected public/config port or actual inode evidence missing")
	}
	operationalRoundTrip(t, net.JoinHostPort("127.0.0.1", portText))
	operationalRoundTrip(t, foreign.Addr().String())
	// Retirement must have its own reviewed, signed identity and actual normal
	// Agent execution; the apply receipt may never be overwritten as a shortcut.
	retireResponse := postIngressOperation(t, endpoint, "retire", spec, tun.PlanHash, "gamma-admin")
	if retireResponse.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(retireResponse.Body)
		retireResponse.Body.Close()
		t.Fatalf("public BCC retire API status=%d body=%s", retireResponse.StatusCode, body)
	}
	var retireResult publicIngressOperationResponse
	if err := json.NewDecoder(retireResponse.Body).Decode(&retireResult); err != nil {
		retireResponse.Body.Close()
		t.Fatal(err)
	}
	retireResponse.Body.Close()
	retiredBeforeRun, ok := s.GetPublicIngressOperation(spec.TunnelID)
	if !ok || retiredBeforeRun.Retirement == nil || retiredBeforeRun.Retirement.Command.JobID != retireResult.JobID || retireResult.ReadyToApply || !retireResult.ReconciliationPending {
		t.Fatalf("public retire API did not queue a distinct reviewed command: result=%+v operation=%+v", retireResult, retiredBeforeRun)
	}
	retire := retiredBeforeRun.Retirement.Command
	checkRun(retire.JobID, "retired")
	retireStatus := readOperationalIngressStatus(t, endpoint, client, spec.TunnelID, "gamma-admin")
	if retireStatus.Action != agentjob.ActionPublicIngressRetire || retireStatus.ReceiptPhase != "retired" || !retireStatus.ReceiptVerified || retireStatus.ReadyToApply || retireStatus.PlanHash != tun.PlanHash || retireStatus.PublicPort != spec.PublicPort {
		t.Fatalf("status API did not expose exact verified Retire receipt safely: %+v", retireStatus)
	}
	retired, ok := s.GetPublicIngressOperation(spec.TunnelID)
	if !ok || retired.Receipt == nil || !reflect.DeepEqual(retired.Receipt, applied.Receipt) {
		t.Fatal("retirement overwrote apply evidence")
	}
	if retired.Retirement == nil || retired.Retirement.Receipt == nil || retired.Retirement.Receipt.ValidateForCommand(retire) != nil || retired.Retirement.Receipt.Phase != "retired" {
		t.Fatalf("actual retirement receipt absent: %+v", retired)
	}
	if conn, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", portText), time.Second); err == nil {
		conn.Close()
		t.Fatal("retired ingress still accepts TCP")
	}
	rules := operationalNFT(t, "-j", "-a", "list", "chain", "inet", "baft", "ingress")
	if bytes.Contains(rules, []byte(ingressauthority.FirewallOwnerTagV1(command.Binding.Port, command.Binding.Proof))) {
		t.Fatal("retired exact-owned nft rule survived")
	}
	if after := operationalNFT(t, "-j", "-a", "list", "table", "inet", "gamma_unrelated"); !bytes.Equal(baseline, after) {
		t.Fatalf("unrelated nft table changed: before=%s after=%s", baseline, after)
	}
	operationalRoundTrip(t, foreign.Addr().String())
	reopened, err := OpenStore(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	persisted, ok := reopened.GetPublicIngressOperation(spec.TunnelID)
	if !ok || persisted.Receipt == nil || persisted.Retirement == nil || persisted.Retirement.Receipt == nil || persisted.Status != PublicIngressPending {
		t.Fatal("restart lost immutable lifecycle evidence or falsely activated config")
	}
}

func requireOperationalLifecycleLAB(t *testing.T) {
	t.Helper()
	if os.Getenv("BAFT_OPERATIONAL_INGRESS_LAB") != "1" {
		t.Skip("requires disposable mount and network isolated operational LAB")
	}
	if os.Geteuid() != 0 {
		t.Fatal("isolated LAB root required")
	}
	for _, kind := range []string{"mnt", "net"} {
		self, err := os.Readlink("/proc/self/ns/" + kind)
		init, initErr := os.Readlink("/proc/1/ns/" + kind)
		if err != nil || initErr != nil || self == init {
			t.Fatalf("private %s namespace required", kind)
		}
	}
	mounts, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/etc/baft", "/etc/baft-agent", "/var/lib/baft-agent"} {
		found := false
		for _, line := range strings.Split(string(mounts), "\n") {
			fields := strings.Fields(line)
			if len(fields) > 5 && fields[4] == path {
				found = true
			}
		}
		marker, err := os.ReadFile(filepath.Join(path, ".signed-lease-lab"))
		if !found || err != nil || string(marker) != "disposable-signed-lease-lab" {
			t.Fatalf("missing private fixed-path mount %s", path)
		}
		local, err := os.Stat(path)
		host, hostErr := os.Stat("/proc/1/root" + path)
		if err != nil || hostErr == nil && os.SameFile(local, host) {
			t.Fatalf("host configuration exposed at %s", path)
		}
	}
}

func configureOperationalLifecycleLAB(t *testing.T, nodeID string, public []byte, foreignEndpoint string) {
	t.Helper()
	ctx := context.Background()
	for _, path := range []string{"/etc/baft/ingress", ingressauthority.InstalledAuthorityDirectory, ingressinventory.InstalledManagedMutationJournalPath} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	// os.TempDir may be below world-writable /tmp; use protected /root ancestry.
	sources, err := os.MkdirTemp("/root", "gamma-operational-sources-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sources) })
	ns, err := os.Readlink("/proc/self/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	policy := ingressinventory.InstalledManagedNodePolicyV1{SchemaVersion: 1, InventoryVersion: ingressinventory.ManagedCoverageVersion, NodeID: nodeID, Platform: "linux", NetworkNamespace: ns, Transport: "dedicated", Manager: "systemd-socket-v1", SourceRoots: []string{sources}}
	raw, err := ingressinventory.MarshalInstalledManagedNodePolicyV1(ctx, policy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ingressinventory.InstalledPolicyPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("/etc/baft-agent/bcc-job.pub", []byte(release.EncodePublic(public)), 0600); err != nil {
		t.Fatal(err)
	}
	operationalNFT(t, "add", "table", "inet", "baft")
	operationalNFT(t, "add", "chain", "inet", "baft", "ingress", "{ type filter hook input priority -100; policy accept; }")
	operationalNFT(t, "add", "table", "inet", "gamma_unrelated")
	operationalNFT(t, "add", "chain", "inet", "gamma_unrelated", "keep")
	_, foreignPort, err := net.SplitHostPort(foreignEndpoint)
	if err != nil {
		t.Fatal(err)
	}
	operationalNFT(t, "add", "rule", "inet", "gamma_unrelated", "keep", "tcp", "dport", foreignPort, "accept")
	raw = operationalNFT(t, "-j", "-a", "list", "chain", "inet", "baft", "ingress")
	var snapshot struct {
		NFTables []struct {
			Chain struct {
				Name   string
				Handle uint64
			}
		}
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	var handle uint64
	for _, entry := range snapshot.NFTables {
		if entry.Chain.Name == "ingress" {
			handle = entry.Chain.Handle
		}
	}
	if handle == 0 {
		t.Fatal("actual kernel nft chain handle missing")
	}
	cfg := ingressinventory.InstalledManagedExecutorConfig{SchemaVersion: 1, NodeID: nodeID, NetworkNamespace: ns, ChainHandle: handle}
	raw, err = json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ingressinventory.InstalledManagedExecutorPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func operationalNFT(t *testing.T, args ...string) []byte {
	t.Helper()
	out, err := exec.Command(ingressinventory.InstalledManagedNFTBinary, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("isolated nft %v: %v %s", args, err, out)
	}
	return out
}

func operationalEchoListener(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
		}
	}()
	t.Cleanup(func() { _ = ln.Close(); <-done })
	return ln
}

func operationalRoundTrip(t *testing.T, endpoint string) {
	t.Helper()
	c, err := net.DialTimeout("tcp4", endpoint, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	want := bytes.Repeat([]byte{0, 255, 42, 13, 10, 128}, 8192)
	if _, err := c.Write(want); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(want))
	if _, err := io.ReadFull(c, got); err != nil || !bytes.Equal(got, want) {
		t.Fatalf("real TCP payload differs: %v", err)
	}
}
