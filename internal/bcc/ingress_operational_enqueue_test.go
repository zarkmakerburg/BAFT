package bcc

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
)

type testIngressProbe struct{ err error }

func (p testIngressProbe) LiveIngressAuthority() error { return p.err }

// openIngressGateForTest installs a probe for one test only. Production code
// has no such setter: the gate is unexported and defaults closed.
func openIngressGateForTest(t *testing.T) {
	t.Helper()
	ingressAuthorityProbe.Store(&ingressProbeBox{probe: testIngressProbe{}})
	t.Cleanup(func() { ingressAuthorityProbe.Store(nil) })
}

func heldIngress(t *testing.T) (*Store, PublicIngressSpec, ingressauthority.CommandV1, string) {
	t.Helper()
	s, spec, _, token := stagedIngressBoundary(t)
	// Whole-second window, as agentjob.Sign emits and the Agent compares.
	s.mu.Lock()
	op := s.st.PublicIngressOperations[spec.TunnelID]
	c := *op.Command
	c.IssuedAt, c.ExpiresAt = c.IssuedAt.UTC().Truncate(time.Second), c.ExpiresAt.UTC().Truncate(time.Second)
	op.Command = &c
	s.st.PublicIngressOperations[spec.TunnelID] = op
	s.mu.Unlock()
	if _, err := s.StageHeldIngressDispatch(spec.TunnelID, time.Now()); err != nil {
		t.Fatal(err)
	}
	return s, spec, c, token
}

func TestOperationalIngressRefusesSubSecondWindowThatAgentCouldNotMatch(t *testing.T) {
	openIngressGateForTest(t)
	s, spec, _, _ := stagedIngressBoundary(t)
	// Staging itself now refuses a lossy window (see the signability tests);
	// keep the enqueue-time defence by simulating a tampered persisted command.
	s.mu.Lock()
	op := s.st.PublicIngressOperations[spec.TunnelID]
	c := *op.Command
	c.IssuedAt = c.IssuedAt.Add(time.Nanosecond)
	op.Command = &c
	s.st.PublicIngressOperations[spec.TunnelID] = op
	s.mu.Unlock()
	if _, err := s.StageHeldIngressDispatch(spec.TunnelID, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.EnqueueOperationalIngress(spec.TunnelID); err == nil || len(s.st.Jobs) != 0 {
		t.Fatalf("sub-second window enqueued: %v", err)
	}
}

func TestExpiredNeverQueuedHeldApplyRestagesWithFreshIdentities(t *testing.T) {
	openIngressGateForTest(t)
	s, spec, original, _ := heldIngress(t)
	now := time.Now().UTC().Truncate(time.Second)
	// Simulate a held command whose signed window expired before it was queued.
	s.mu.Lock()
	op := s.st.PublicIngressOperations[spec.TunnelID]
	cmd := *op.Command
	cmd.IssuedAt = now.Add(-25 * time.Hour).Truncate(time.Second)
	cmd.ExpiresAt = now.Add(-time.Second).Truncate(time.Second)
	op.Command = &cmd
	s.st.PublicIngressOperations[spec.TunnelID] = op
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()
	if err := s.enqueueOperationalIngress(spec.TunnelID, now); err == nil {
		t.Fatal("first expired enqueue must durably expire, not dispatch, the old command")
	}
	old, _ := s.GetPublicIngressOperation(spec.TunnelID)
	if old.Dispatch == nil || old.Dispatch.Phase != ingressPhaseExpired || len(s.ListJobs()) != 0 {
		t.Fatalf("expired held command was not safely fenced: dispatch=%+v jobs=%+v", old.Dispatch, s.ListJobs())
	}
	// Automatic activation/poll retries must not mint a replacement command.
	if err := s.EnqueueOperationalIngress(spec.TunnelID); err == nil {
		t.Fatal("automatic retry silently restaged an expired command without review")
	}
	afterAutomaticRetry, _ := s.GetPublicIngressOperation(spec.TunnelID)
	if afterAutomaticRetry.OperationID != old.OperationID || afterAutomaticRetry.Dispatch.Phase != ingressPhaseExpired || len(s.ListJobs()) != 0 {
		t.Fatalf("automatic retry changed an expired operation: before=%+v after=%+v jobs=%+v", old, afterAutomaticRetry, s.ListJobs())
	}
	// Only the explicit reviewed-operation route may safely restage it.
	if err := s.EnqueueReviewedOperationalIngress(spec.TunnelID); err != nil {
		t.Fatalf("reviewed fresh restage and enqueue: %v", err)
	}
	got, _ := s.GetPublicIngressOperation(spec.TunnelID)
	if got.OperationID == old.OperationID || got.LeaseID == old.LeaseID || got.Command.JobID == old.Command.JobID ||
		got.Command.JobID == original.JobID || got.Dispatch.Phase != ingressPhaseQueued {
		t.Fatalf("restage did not use fresh identities and queue the new command: old=%+v new=%+v", old, got)
	}
	if len(s.ListJobs()) != 1 {
		t.Fatalf("expected one queued Apply, got %+v", s.ListJobs())
	}
	if err := validatePersistedIngressOperations(s.st); err != nil {
		t.Fatalf("restaged operation failed durable validation: %v", err)
	}
}

func TestDecommissionedTunnelRetainedIngressOperationDoesNotBlockStoreRestart(t *testing.T) {
	s, spec, _, _ := heldIngress(t)
	s.mu.Lock()
	tun := s.st.Tunnels[spec.TunnelID]
	tun.Phase = TunnelDecommissioned
	s.st.Tunnels[spec.TunnelID] = tun
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()
	if err := validatePersistedIngressOperations(s.st); err != nil {
		t.Fatalf("retained terminal ingress history should validate: %v", err)
	}
	if _, err := OpenStore(s.Path()); err != nil {
		t.Fatalf("decommissioned tunnel with retained ingress operation blocked BCC restart: %v", err)
	}
}

func TestOperationalIngressEnqueueGateDefaultsClosed(t *testing.T) {
	s, spec, _, _ := heldIngress(t)
	if err := s.EnqueueOperationalIngress(spec.TunnelID); !errors.Is(err, ErrIngressAuthorityUnavailable) {
		t.Fatalf("gate not closed by default: %v", err)
	}
	if err := s.EnqueueOperationalIngressRetire(spec.TunnelID); !errors.Is(err, ErrIngressAuthorityUnavailable) {
		t.Fatalf("retire gate not closed by default: %v", err)
	}
	ingressAuthorityProbe.Store(&ingressProbeBox{probe: testIngressProbe{err: errors.New("no lease")}})
	defer ingressAuthorityProbe.Store(nil)
	if err := s.EnqueueOperationalIngress(spec.TunnelID); !errors.Is(err, ErrIngressAuthorityUnavailable) {
		t.Fatalf("failing probe opened gate: %v", err)
	}
	if len(s.st.Jobs) != 0 {
		t.Fatal("closed gate queued a job")
	}
}

func TestOperationalIngressEnqueueSignsReviewedCommandAndIsRetrySafe(t *testing.T) {
	openIngressGateForTest(t)
	s, spec, cmd, _ := heldIngress(t)
	if err := s.EnqueueOperationalIngress(spec.TunnelID); err != nil {
		t.Fatal(err)
	}
	before := stateJSONForTest(t, s)
	if err := s.EnqueueOperationalIngress(spec.TunnelID); err != nil {
		t.Fatalf("identical retry: %v", err)
	}
	if string(stateJSONForTest(t, s)) != string(before) {
		t.Fatal("retry changed durable state")
	}
	reopened, err := OpenStore(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	op, _ := reopened.GetPublicIngressOperation(spec.TunnelID)
	if op.Dispatch.Phase != "queued" || op.Status != PublicIngressPending || op.Receipt != nil {
		t.Fatalf("durable dispatch: %+v", op.Dispatch)
	}
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	app := &Server{store: reopened, jobKey: key, now: time.Now}
	jobs, err := reopened.pullJobs(spec.IRNode, "ingress-receipt-enrolled-agent", func(js []Job) error {
		signed, err := app.signAgentJobs(js)
		if err != nil || len(signed) != 1 || signed[0].Signed == nil {
			t.Fatalf("signing: %v %d", err, len(signed))
		}
		got, err := (agentjob.Verifier{BCCKey: pub, NodeID: spec.IRNode}).Verify(*signed[0].Signed, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if got.JobID != cmd.JobID || got.Action != agentjob.ActionPublicIngressApply || !got.IssuedAt.Equal(cmd.IssuedAt) || !got.ExpiresAt.Equal(cmd.ExpiresAt) ||
			got.Params["plan_sha256"] != cmd.Binding.PlanSHA256 || got.Params["operation_id"] != cmd.Binding.OperationID || got.Params["lease_id"] != cmd.Binding.LeaseID ||
			got.Params["public_port"] != got.Params["listen_endpoint"][len("0.0.0.0:"):] {
			t.Fatalf("signed job differs from reviewed command: %+v", got)
		}
		return nil
	})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("pull: %v %d", err, len(jobs))
	}
}

func TestOperationalIngressEnqueueSaveFailureCompensates(t *testing.T) {
	openIngressGateForTest(t)
	s, spec, _, _ := heldIngress(t)
	before := stateJSONForTest(t, s)
	path := s.path
	s.path = t.TempDir()
	if err := s.EnqueueOperationalIngress(spec.TunnelID); err == nil {
		t.Fatal("unwritable enqueue committed")
	}
	s.path = path
	if string(stateJSONForTest(t, s)) != string(before) || len(s.st.Jobs) != 0 {
		t.Fatal("failed enqueue left job or phase change")
	}
}

func TestOperationalIngressExpiredCommandCompensatesAndCannotReplay(t *testing.T) {
	openIngressGateForTest(t)
	s, spec, cmd, _ := heldIngress(t)
	if err := s.enqueueOperationalIngress(spec.TunnelID, cmd.ExpiresAt.Add(time.Second)); err == nil {
		t.Fatal("expired command enqueued")
	}
	op, _ := s.GetPublicIngressOperation(spec.TunnelID)
	if op.Dispatch.Phase != "expired" || len(s.st.Jobs) != 0 {
		t.Fatalf("expiry not compensated: %+v", op.Dispatch)
	}
	if err := s.EnqueueOperationalIngress(spec.TunnelID); err == nil {
		t.Fatal("expired dispatch replayed")
	}
}

func TestOperationalIngressQueuedJobExpiresBeforeDispatchAndFailedAckCompensates(t *testing.T) {
	openIngressGateForTest(t)
	s, spec, cmd, _ := heldIngress(t)
	if err := s.EnqueueOperationalIngress(spec.TunnelID); err != nil {
		t.Fatal(err)
	}
	// Dispatch, then the Agent acknowledges failure.
	jobs, err := s.PullJobs(spec.IRNode, "ingress-receipt-enrolled-agent")
	if err != nil || len(jobs) != 1 {
		t.Fatalf("pull: %v %d", err, len(jobs))
	}
	if err := s.AckJob(spec.IRNode, "ingress-receipt-enrolled-agent", cmd.JobID, "failed", "x"); err != nil {
		t.Fatal(err)
	}
	op, _ := s.GetPublicIngressOperation(spec.TunnelID)
	if op.Dispatch.Phase != "failed" || op.Status != PublicIngressPending {
		t.Fatalf("failed ack: %+v", op.Dispatch)
	}
	if err := s.EnqueueOperationalIngress(spec.TunnelID); err == nil {
		t.Fatal("failed dispatch replayed")
	}
	if err := validatePersistedIngressOperations(s.st); err != nil {
		t.Fatal(err)
	}
}

func TestOperationalIngressQueuedPastWindowIsExpiredNotSigned(t *testing.T) {
	openIngressGateForTest(t)
	s, spec, cmd, _ := heldIngress(t)
	if err := s.EnqueueOperationalIngress(spec.TunnelID); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	op := s.st.PublicIngressOperations[spec.TunnelID]
	c := *op.Command
	c.IssuedAt, c.ExpiresAt = time.Now().Add(-2*time.Minute), time.Now().Add(-time.Minute)
	op.Command = &c
	s.st.PublicIngressOperations[spec.TunnelID] = op
	s.mu.Unlock()
	jobs, err := s.PullJobs(spec.IRNode, "ingress-receipt-enrolled-agent")
	if err != nil || len(jobs) != 0 {
		t.Fatalf("expired job dispatched: %v %d", err, len(jobs))
	}
	got, _ := s.GetPublicIngressOperation(spec.TunnelID)
	if got.Dispatch.Phase != "expired" || s.st.Jobs[cmd.JobID].Status != "failed" {
		t.Fatalf("not compensated: %+v %+v", got.Dispatch, s.st.Jobs[cmd.JobID])
	}
}

func TestOperationalIngressRetireRequiresExactAppliedReceipt(t *testing.T) {
	openIngressGateForTest(t)
	s, spec, cmd, token := heldIngress(t)
	if err := s.EnqueueOperationalIngress(spec.TunnelID); err != nil {
		t.Fatal(err)
	}
	if err := s.EnqueueOperationalIngressRetire(spec.TunnelID); err == nil {
		t.Fatal("retire without receipt")
	}
	blocked := ingressauthority.ReceiptV1{SchemaVersion: 1, JobID: cmd.JobID, Action: cmd.Action, Binding: cmd.Binding, Phase: "blocked", RollbackStatus: "not_attempted"}
	if err := s.RecordIngressAgentReceipt(blocked, token, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.EnqueueOperationalIngressRetire(spec.TunnelID); err == nil {
		t.Fatal("retire after blocked receipt")
	}
	s.mu.Lock()
	op := s.st.PublicIngressOperations[spec.TunnelID]
	applied := appliedReceipt(cmd)
	op.Receipt = &applied
	s.st.PublicIngressOperations[spec.TunnelID] = op
	s.mu.Unlock()
	if err := s.EnqueueOperationalIngressRetire(spec.TunnelID); err != nil {
		t.Fatal(err)
	}
	first, _ := s.GetPublicIngressOperation(spec.TunnelID)
	if first.RetireCommand == nil || first.RetireCommand.JobID == cmd.JobID || first.RetireCommand.Action != "public_ingress_retire" || first.RetireCommand.Binding != cmd.Binding {
		t.Fatalf("retire command: %+v", first.RetireCommand)
	}
	if err := s.EnqueueOperationalIngressRetire(spec.TunnelID); err != nil {
		t.Fatalf("retire retry: %v", err)
	}
	again, _ := s.GetPublicIngressOperation(spec.TunnelID)
	if again.RetireCommand.JobID != first.RetireCommand.JobID {
		t.Fatal("retire retry minted a second JobID")
	}
	if err := validatePersistedIngressOperations(s.st); err != nil {
		t.Fatal(err)
	}
	// Retire receipt is inert evidence and never REMOVED.
	rr := ingressauthority.ReceiptV1{SchemaVersion: 1, JobID: first.RetireCommand.JobID, Action: "public_ingress_retire", Binding: cmd.Binding, Phase: "retired", OwnershipVerified: true}
	if err := s.RecordIngressAgentReceipt(rr, token, time.Now()); err != nil {
		t.Fatal(err)
	}
	done, _ := s.GetPublicIngressOperation(spec.TunnelID)
	if done.Status != PublicIngressPending || done.RetireReceipt == nil {
		t.Fatalf("retire receipt: %+v", done)
	}
}

func appliedReceipt(cmd ingressauthority.CommandV1) ingressauthority.ReceiptV1 {
	return ingressauthority.ReceiptV1{SchemaVersion: 1, JobID: cmd.JobID, Action: cmd.Action, Binding: cmd.Binding, Phase: "applied", OwnershipVerified: true,
		ListenerEvidence: ingressauthority.ListenerEvidenceV1{Endpoint: cmd.Binding.ListenEndpoint, Family: "tcp4", SocketIdentity: "socket:12345"},
		FirewallEvidence: []ingressauthority.FirewallEvidenceV1{{Family: "ip", Table: "baft", Chain: "ingress", ChainHandle: 3, RuleHandle: 9, OwnerTag: "baft:" + cmd.Binding.OperationID}}}
}

func TestIngressReadinessOnlyAfterExactReceiptAndIndependentEvidence(t *testing.T) {
	openIngressGateForTest(t)
	s, spec, cmd, token := heldIngress(t)
	now := time.Now().UTC()
	obs := &IndependentIngressObservation{Binding: cmd.Binding, ObservedAt: now, ListenerEndpoint: cmd.Binding.ListenEndpoint, SocketIdentity: "socket:12345", ChainHandle: 3, RuleHandle: 9, TCPPayloadVerified: true}
	if r := s.EvaluateIngressReadiness(spec.TunnelID, obs, now); r.Ready || r.Card != nil {
		t.Fatal("desired/held state reported ready")
	}
	if err := s.EnqueueOperationalIngress(spec.TunnelID); err != nil {
		t.Fatal(err)
	}
	if r := s.EvaluateIngressReadiness(spec.TunnelID, obs, now); r.Ready {
		t.Fatal("queued job without receipt reported ready")
	}
	if err := s.RecordIngressAgentReceipt(appliedReceipt(cmd), token, now); err != nil {
		t.Fatal(err)
	}
	if r := s.EvaluateIngressReadiness(spec.TunnelID, nil, now); r.Ready || r.Verified {
		t.Fatal("receipt alone reported ready")
	}
	for name, mut := range map[string]func(*IndependentIngressObservation){
		"stale":   func(o *IndependentIngressObservation) { o.ObservedAt = now.Add(-time.Hour) },
		"socket":  func(o *IndependentIngressObservation) { o.SocketIdentity = "socket:other" },
		"handle":  func(o *IndependentIngressObservation) { o.RuleHandle = 10 },
		"payload": func(o *IndependentIngressObservation) { o.TCPPayloadVerified = false },
		"binding": func(o *IndependentIngressObservation) { o.Binding.LeaseID = "other-lease" },
	} {
		bad := *obs
		mut(&bad)
		if r := s.EvaluateIngressReadiness(spec.TunnelID, &bad, now); r.Ready || r.Card != nil {
			t.Fatalf("%s: mismatched evidence reported ready", name)
		}
	}
	r := s.EvaluateIngressReadiness(spec.TunnelID, obs, now)
	if !r.Ready || !r.Verified || r.Card == nil || r.Card.ConfigPort != r.Card.PublicPort || int(r.Card.PublicPort) != spec.PublicPort {
		t.Fatalf("exact evidence not ready: %+v", r)
	}
}

func TestIngressReadinessRefusesBlockedAndSyntheticReceipts(t *testing.T) {
	openIngressGateForTest(t)
	s, spec, cmd, token := heldIngress(t)
	now := time.Now().UTC()
	if err := s.EnqueueOperationalIngress(spec.TunnelID); err != nil {
		t.Fatal(err)
	}
	synthetic := appliedReceipt(cmd)
	synthetic.OwnershipVerified = false
	if err := s.RecordIngressAgentReceipt(synthetic, token, now); err != nil {
		t.Fatal(err)
	}
	obs := &IndependentIngressObservation{Binding: cmd.Binding, ObservedAt: now, ListenerEndpoint: cmd.Binding.ListenEndpoint, SocketIdentity: "socket:12345", ChainHandle: 3, RuleHandle: 9, TCPPayloadVerified: true}
	if r := s.EvaluateIngressReadiness(spec.TunnelID, obs, now); r.Ready {
		t.Fatal("unverified-ownership receipt reported ready")
	}
}

func TestFailedWizardCreateMakesNoDurableJobsOrIngressIntent(t *testing.T) {
	s := tunnelStore(t)
	now := time.Now().UTC()
	for _, req := range []TunnelRequest{
		{EXNode: "ex-1", IRNode: "ir-1", ClientPort: 22},
		{EXNode: "ex-1", IRNode: "ir-1", ClientPort: 1443}, // aliases the default IR route listener
		{EXNode: "ex-1", IRNode: "missing", ClientPort: 2053},
	} {
		if _, err := s.BuildPlan(req, now); err == nil {
			// A plan that builds must still refuse a stale reviewed hash below.
			if _, _, err := s.CreateTunnelFromPlan(req, "stale-plan-hash", now); err == nil {
				t.Fatalf("stale reviewed plan created tunnel for %+v", req)
			}
		} else if _, err := s.CreateTunnel(req, now); err == nil {
			t.Fatalf("invalid request created tunnel: %+v", req)
		}
		if len(s.st.Jobs) != 0 || len(s.st.Tunnels) != 0 || len(s.st.PublicIngressOperations) != 0 || len(s.st.PublicIngressReservations) != 0 {
			t.Fatalf("failed request left durable state: jobs=%d tunnels=%d ops=%d res=%d", len(s.st.Jobs), len(s.st.Tunnels), len(s.st.PublicIngressOperations), len(s.st.PublicIngressReservations))
		}
	}
}
