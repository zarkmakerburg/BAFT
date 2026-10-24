package bcc

import (
	"errors"
	"github.com/zarkmakerburg/baft/internal/ingressauthority"
	"sync"
	"testing"
	"time"
)

func TestIngressDispatchIntentHeldAcrossRestartAndExactRetry(t *testing.T) {
	s, spec, cmd, token := stagedIngressBoundary(t)
	now := time.Now().UTC()
	first, err := s.StageHeldIngressDispatch(spec.TunnelID, now)
	if err != nil {
		t.Fatal(err)
	}
	if first.JobID != cmd.JobID || first.Phase != "held" || first.OperationID != cmd.Binding.OperationID || first.LeaseID != cmd.Binding.LeaseID {
		t.Fatalf("wrong immutable dispatch intent: %+v", first)
	}
	reopened, err := OpenStore(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	op := reopened.st.PublicIngressOperations[spec.TunnelID]
	if op.Dispatch == nil || *op.Dispatch != first || op.Receipt != nil || op.Status != PublicIngressPending {
		t.Fatalf("lost held identity or falsely completed: %+v", op)
	}
	before := stateJSONForTest(t, s)
	retry, err := s.StageHeldIngressDispatch(spec.TunnelID, now.Add(time.Minute))
	if err != nil || retry != first {
		t.Fatalf("identical retry changed intent: %+v %v", retry, err)
	}
	if got := stateJSONForTest(t, s); string(got) != string(before) {
		t.Fatal("retry modified durable state")
	}
	if err := s.EnqueueOperationalIngress(spec.TunnelID); !errors.Is(err, ErrIngressAuthorityUnavailable) {
		t.Fatalf("operational dispatch unexpectedly enabled: %v", err)
	}
	if len(s.st.Jobs) != 0 {
		t.Fatal("held intent became executable agent job")
	}
	receipt := ingressauthority.ReceiptV1{SchemaVersion: 1, JobID: cmd.JobID, Action: cmd.Action, Binding: cmd.Binding, Phase: "blocked", OwnershipVerified: false, RollbackStatus: "not_attempted"}
	if err := s.RecordIngressAgentReceipt(receipt, token, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StageHeldIngressDispatch(spec.TunnelID, now.Add(time.Minute)); err == nil {
		t.Fatal("blocked receipt permits new dispatch staging")
	}
	if err := s.EnqueueOperationalIngress(spec.TunnelID); !errors.Is(err, ErrIngressAuthorityUnavailable) {
		t.Fatal("blocked receipt promoted to executable job")
	}
	op, _ = s.GetPublicIngressOperation(spec.TunnelID)
	if op.Status != PublicIngressPending || op.Dispatch.Phase != "held" {
		t.Fatal("blocked outcome turned into success")
	}
}

func TestIngressDispatchIntentRefusesUnstagedAndStaleIdentity(t *testing.T) {
	s, spec, _, _ := stagedIngressBoundary(t)
	now := time.Now().UTC()
	s.mu.Lock()
	op := s.st.PublicIngressOperations[spec.TunnelID]
	op.Command = nil
	s.st.PublicIngressOperations[spec.TunnelID] = op
	s.mu.Unlock()
	if _, err := s.StageHeldIngressDispatch(spec.TunnelID, now); err == nil {
		t.Fatal("missing staged command accepted")
	}
	if len(s.st.Jobs) != 0 {
		t.Fatal("unstaged operation created job")
	}
}

func TestIngressDispatchIntentTamperedRestoreFailsClosed(t *testing.T) {
	s, spec, _, _ := stagedIngressBoundary(t)
	if _, err := s.StageHeldIngressDispatch(spec.TunnelID, time.Now()); err != nil {
		t.Fatal(err)
	}
	for _, alter := range []func(*IngressDispatchIntent){
		func(i *IngressDispatchIntent) { i.JobID = "another-job" },
		func(i *IngressDispatchIntent) { i.OperationID = "another-operation" },
		func(i *IngressDispatchIntent) { i.Phase = "queued" },
		func(i *IngressDispatchIntent) { i.LeaseID = "another-lease" },
	} {
		snapshot, err := s.snapshotState()
		if err != nil {
			t.Fatal(err)
		}
		op := snapshot.PublicIngressOperations[spec.TunnelID]
		copy := *op.Dispatch
		alter(&copy)
		op.Dispatch = &copy
		snapshot.PublicIngressOperations[spec.TunnelID] = op
		if err := validatePersistedIngressOperations(snapshot); err == nil {
			t.Fatal("tampered dispatch intent accepted")
		}
		if err := mergePublicIngressOperations(&snapshot, s.st); err == nil {
			t.Fatal("restore accepted detached intent")
		}
	}
}

func TestIngressDispatchStageSaveFailureAndExistingQueueCollision(t *testing.T) {
	s, spec, cmd, _ := stagedIngressBoundary(t)
	now := time.Now().UTC()
	s.mu.Lock()
	s.st.Jobs["queue-collision"] = Job{ID: cmd.JobID, Type: "deploy_baft", NodeID: spec.IRNode}
	s.mu.Unlock()
	if _, err := s.StageHeldIngressDispatch(spec.TunnelID, now); err == nil {
		t.Fatal("staged held intent shares runnable job ID")
	}
	s.mu.Lock()
	delete(s.st.Jobs, "queue-collision")
	s.mu.Unlock()
	before := stateJSONForTest(t, s)
	path := s.path
	s.path = t.TempDir()
	if _, err := s.StageHeldIngressDispatch(spec.TunnelID, now); err == nil {
		t.Fatal("unwritable dispatch stage committed")
	}
	s.path = path
	if string(stateJSONForTest(t, s)) != string(before) {
		t.Fatal("failed save altered held intent/command")
	}
}

func TestIngressDispatchStageConcurrentImmutableRetries(t *testing.T) {
	s, spec, _, _ := stagedIngressBoundary(t)
	now := time.Now().UTC()
	results := make(chan error, 12)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.StageHeldIngressDispatch(spec.TunnelID, now); results <- err }()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	op, ok := s.GetPublicIngressOperation(spec.TunnelID)
	if !ok || op.Dispatch == nil || op.Dispatch.Phase != "held" || op.Status != PublicIngressPending {
		t.Fatal("concurrent retries lost held fence")
	}
	reopened, err := OpenStore(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if reopened.st.PublicIngressOperations[spec.TunnelID].Dispatch == nil {
		t.Fatal("held staging not durable")
	}
}
