package bcc

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"

	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

func stagedIngressBoundary(t *testing.T) (*Store, PublicIngressSpec, ingressauthority.CommandV1, string) {
	t.Helper()
	s, spec := reservationStore(t)
	token := "ingress-receipt-enrolled-agent"
	s.mu.Lock()
	tun := s.st.Tunnels[spec.TunnelID]
	tun.PlanHash = strings.Repeat("a", 64)
	tun.ClientPort = spec.PublicPort
	s.st.Tunnels[tun.ID] = tun
	node := s.st.Nodes[spec.IRNode]
	node.AgentTokenHash = tokenHash(token)
	s.st.Nodes[spec.IRNode] = node
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()
	proof := canonicalOperationProof(t, s, spec, 1)
	op, err := s.RecordReviewedPublicIngressOperation(spec, tun.PlanHash, proof, time.Now(), ingressAudit())
	if err != nil {
		t.Fatal(err)
	}
	if op.OperationID == "" || op.LeaseID == "" {
		t.Fatal("durable operation / lease identity missing")
	}
	manifest := publicIngressManifestSpec(spec, tun.InstanceID, proof.Generation)
	preview, err := ingressmanifest.Preview(manifest)
	if err != nil {
		t.Fatal(err)
	}
	bind := ingressauthority.BindingV1{
		NodeID: spec.IRNode, OperationID: op.OperationID, LeaseID: op.LeaseID,
		Port: uint16(spec.PublicPort), ConfigPort: uint16(spec.PublicPort),
		ListenEndpoint: manifest.Config.Listen, LocalTarget: manifest.Config.Target,
		Transport: "dedicated", PlanSHA256: tun.PlanHash, ListenerID: proof.ListenerID, FirewallID: proof.FirewallID,
		Spec: manifest, Proof: preview,
	}
	now := time.Now().UTC().Truncate(time.Second)
	cmd := ingressauthority.CommandV1{IngressProtocol: ingressauthority.ProtocolV1, JobID: "agent-job-reviewed-1", Action: "public_ingress_apply", IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Binding: bind}
	if err := s.StageReviewedIngressCommand(spec.TunnelID, cmd, now); err != nil {
		t.Fatal(err)
	}
	return s, spec, cmd, token
}

func TestStagedIngressCommandDurableAndRejectsDifferentBinding(t *testing.T) {
	s, spec, cmd, _ := stagedIngressBoundary(t)
	reopened, err := OpenStore(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	saved := reopened.st.PublicIngressOperations[spec.TunnelID]
	if saved.Command == nil || *saved.Command != cmd || saved.OperationID != cmd.Binding.OperationID || saved.LeaseID != cmd.Binding.LeaseID {
		t.Fatalf("command identity not stable after restart: %+v", saved)
	}
	mutated := cmd
	mutated.Binding.ConfigPort++
	if err := s.StageReviewedIngressCommand(spec.TunnelID, mutated, time.Now()); err == nil {
		t.Fatal("wrong config port command admitted")
	}
	mutated = cmd
	mutated.JobID = "replayed-or-different-job"
	if err := s.StageReviewedIngressCommand(spec.TunnelID, mutated, time.Now()); err == nil {
		t.Fatal("second command replaced durable signed identity")
	}
}

func TestIngressReceiptReceiverRequiresHTTPSCredentialAndExactCommand(t *testing.T) {
	s, spec, cmd, token := stagedIngressBoundary(t)
	app, err := NewServer(s, "admin")
	if err != nil {
		t.Fatal(err)
	}
	receipt := ingressauthority.ReceiptV1{SchemaVersion: 1, JobID: cmd.JobID, Action: cmd.Action, Binding: cmd.Binding, Phase: "applied", OwnershipVerified: true}
	invoke := func(useTLS bool, auth string, body any) *httptest.ResponseRecorder {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "https://bcc.example/api/ingress/operational-receipt", strings.NewReader(string(raw)))
		req.TLS = nil
		if useTLS {
			req.TLS = &tls.ConnectionState{}
		}
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		rr := httptest.NewRecorder()
		app.Handler().ServeHTTP(rr, req)
		return rr
	}
	if rr := invoke(false, token, receipt); rr.Code != http.StatusUpgradeRequired {
		t.Fatalf("plaintext ingress accepted: %d %s", rr.Code, rr.Body.String())
	}
	if rr := invoke(true, "wrong", receipt); rr.Code != http.StatusUnauthorized {
		t.Fatalf("wrong credential accepted: %d %s", rr.Code, rr.Body.String())
	}
	wrong := receipt
	wrong.JobID = "other-job"
	if rr := invoke(true, token, wrong); rr.Code != http.StatusConflict {
		t.Fatalf("wrong job accepted: %d %s", rr.Code, rr.Body.String())
	}
	wrong = receipt
	wrong.Binding.PlanSHA256 = strings.Repeat("b", 64)
	if rr := invoke(true, token, wrong); rr.Code != http.StatusConflict {
		t.Fatalf("wrong plan accepted: %d %s", rr.Code, rr.Body.String())
	}
	wrong = receipt
	wrong.Action = "public_ingress_retire"
	if rr := invoke(true, token, wrong); rr.Code != http.StatusConflict {
		t.Fatalf("wrong action accepted: %d %s", rr.Code, rr.Body.String())
	}
	rr := invoke(true, token, receipt)
	if rr.Code != http.StatusAccepted || !strings.Contains(rr.Body.String(), "\"verified\":false") || !strings.Contains(rr.Body.String(), "\"ready_to_apply\":false") {
		t.Fatalf("unsupported executor falsely confirmed: %d %s", rr.Code, rr.Body.String())
	}
	reopened, err := OpenStore(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	op := reopened.st.PublicIngressOperations[spec.TunnelID]
	if op.Status != PublicIngressPending || op.Receipt == nil || op.Receipt.JobID != cmd.JobID {
		t.Fatalf("receipt not durably retained as unverified pending evidence: %+v", op)
	}
}

func TestIngressReceiptIdenticalRetryAndDetachedMutation(t *testing.T) {
	s, spec, cmd, token := stagedIngressBoundary(t)
	receipt := ingressauthority.ReceiptV1{
		SchemaVersion: 1, JobID: cmd.JobID, Action: cmd.Action, Binding: cmd.Binding,
		Phase: "applied", OwnershipVerified: true,
		FirewallEvidence: []ingressauthority.FirewallEvidenceV1{{Family: "inet", Table: "baft", Chain: "ingress", OwnerTag: "owned"}},
	}
	now := time.Now()
	if err := s.RecordIngressAgentReceipt(receipt, token, now); err != nil {
		t.Fatal(err)
	}
	before, err := s.snapshotState()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordIngressAgentReceipt(receipt, token, now); err != nil {
		t.Fatalf("identical retry must be idempotent: %v", err)
	}
	if err := s.RecordIngressAgentReceipt(receipt, token, cmd.ExpiresAt.Add(time.Second)); err != nil {
		t.Fatalf("late identical retry must not create new authority or fail reconciliation: %v", err)
	}
	after, err := s.snapshotState()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("idempotent retry modified persisted state")
	}
	receipt.FirewallEvidence[0].OwnerTag = "mutated by caller"
	got, ok := s.GetPublicIngressOperation(spec.TunnelID)
	if !ok || got.Receipt == nil || got.Receipt.FirewallEvidence[0].OwnerTag != "owned" {
		t.Fatal("original caller changed internal receipt")
	}
	got.Command.JobID = "altered"
	got.Receipt.Binding.OperationID = "altered"
	got.Receipt.FirewallEvidence[0].OwnerTag = "altered"
	actual, ok := s.GetPublicIngressOperation(spec.TunnelID)
	if !ok || actual.Command.JobID != cmd.JobID || actual.Receipt.Binding.OperationID != cmd.Binding.OperationID || actual.Receipt.FirewallEvidence[0].OwnerTag != "owned" {
		t.Fatal("getter exposed mutable operation pointers")
	}
	if actual.Status != PublicIngressPending {
		t.Fatal("receipt falsely marked verified")
	}
}

func TestPersistedIngressCommandReceiptRefusesTamperedStateAndRestore(t *testing.T) {
	s, spec, cmd, token := stagedIngressBoundary(t)
	receipt := ingressauthority.ReceiptV1{SchemaVersion: 1, JobID: cmd.JobID, Action: cmd.Action, Binding: cmd.Binding, Phase: "applied"}
	if err := s.RecordIngressAgentReceipt(receipt, token, time.Now()); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		alter func(*PublicIngressOperation)
	}{
		{"operation ID", func(op *PublicIngressOperation) { op.OperationID = "op-other" }},
		{"lease ID", func(op *PublicIngressOperation) { op.LeaseID = "lease-other" }},
		{"command job", func(op *PublicIngressOperation) { op.Command.JobID = "bad job" }},
		{"command plan", func(op *PublicIngressOperation) { op.Command.Binding.PlanSHA256 = strings.Repeat("b", 64) }},
		{"command node", func(op *PublicIngressOperation) { op.Command.Binding.NodeID = "foreign" }},
		{"command port", func(op *PublicIngressOperation) { op.Command.Binding.ConfigPort++ }},
		{"command generation", func(op *PublicIngressOperation) { op.Command.Binding.Spec.Generation++ }},
		{"receipt job", func(op *PublicIngressOperation) { op.Receipt.JobID = "changed" }},
		{"receipt binding", func(op *PublicIngressOperation) { op.Receipt.Binding.OperationID = "other" }},
		{"receipt schema", func(op *PublicIngressOperation) { op.Receipt.SchemaVersion = 99 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy, err := s.snapshotState()
			if err != nil {
				t.Fatal(err)
			}
			op := copy.PublicIngressOperations[spec.TunnelID]
			tc.alter(&op)
			copy.PublicIngressOperations[spec.TunnelID] = op
			if err := validatePersistedIngressOperations(copy); err == nil {
				t.Fatal("invalid persisted ingress operation accepted")
			}
			if err := mergePublicIngressOperations(&copy, s.st); err == nil {
				t.Fatal("tampered backup operation accepted")
			}
		})
	}
}

func TestOpenStoreRefusesTamperedIngressCommandFromDisk(t *testing.T) {
	s, spec, _, _ := stagedIngressBoundary(t)
	original, err := s.snapshotState()
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	op := s.st.PublicIngressOperations[spec.TunnelID]
	changed := *op.Command
	changed.Binding.PlanSHA256 = strings.Repeat("b", 64)
	op.Command = &changed
	s.st.PublicIngressOperations[spec.TunnelID] = op
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()
	if _, err := OpenStore(s.Path()); err == nil {
		t.Fatal("OpenStore accepted mismatching command")
	}
	s.mu.Lock()
	s.st = original
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()
	if _, err := OpenStore(s.Path()); err != nil {
		t.Fatalf("valid state did not reopen: %v", err)
	}
}
