package bcc

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/zarkmakerburg/baft/internal/agentjob"
)

type deployReq struct {
	TunnelID string `json:"tunnel_id"`
	PlanHash string `json:"plan_hash"`
}

func TestIngressDeployAPIStagesHeldThenGateQueuesOnceAndNeverClaimsReady(t *testing.T) {
	f := newTunnelHTTPFixture(t)
	tn := glueTunnel(f, 54619) // glue disabled in tests: nothing staged yet
	if _, ok := f.store.GetPublicIngressOperation(tn.ID); ok {
		t.Fatal("precondition: nothing staged")
	}
	f.call(http.MethodGet, "/api/ingress/deploy?tunnel_id="+url.QueryEscape(tn.ID), tunnelHTTPAdmin, nil, http.StatusNotFound, nil)

	// Authentication and exact-plan checks come first.
	f.call(http.MethodPost, "/api/ingress/deploy", "", deployReq{tn.ID, tn.PlanHash}, http.StatusUnauthorized, nil)
	f.call(http.MethodPost, "/api/ingress/deploy", tunnelHTTPAdmin, deployReq{tn.ID, strings.Repeat("b", 64)}, http.StatusConflict, nil)
	f.call(http.MethodPost, "/api/ingress/deploy", tunnelHTTPAdmin, map[string]string{"tunnel_id": tn.ID}, http.StatusBadRequest, nil)
	f.call(http.MethodPost, "/api/ingress/deploy", tunnelHTTPAdmin, map[string]string{"tunnel_id": tn.ID, "plan_hash": tn.PlanHash, "extra": "x"}, http.StatusBadRequest, nil)
	if _, ok := f.store.GetPublicIngressOperation(tn.ID); ok || applyJobs(f.store) != 0 {
		t.Fatal("a refused request staged state")
	}

	// Closed gate: staged and HELD, never queued, never Ready.
	var held publicIngressOperationResponse
	f.call(http.MethodPost, "/api/ingress/deploy", tunnelHTTPAdmin, deployReq{tn.ID, tn.PlanHash}, http.StatusAccepted, &held)
	if held.State != "held" || held.DispatchPhase != ingressPhaseHeld || held.ReadyToApply || held.ReceiptVerified ||
		held.PlanHash != tn.PlanHash || held.PublicPort != 54619 || held.Action != agentjob.ActionPublicIngressApply || held.JobID == "" {
		t.Fatalf("closed gate response overstated: %+v", held)
	}
	if applyJobs(f.store) != 0 {
		t.Fatal("deploy created a job while the gate was closed")
	}
	var again publicIngressOperationResponse
	f.call(http.MethodPost, "/api/ingress/deploy", tunnelHTTPAdmin, deployReq{tn.ID, tn.PlanHash}, http.StatusAccepted, &again)
	if again.JobID != held.JobID || again.OperationID != held.OperationID {
		t.Fatalf("retry changed the immutable command identity: %+v vs %+v", again, held)
	}

	// Capability heartbeat opens the gate; a second deploy is idempotent.
	f.pullCaps("ir-1", testIngressCapability)
	var queued publicIngressOperationResponse
	f.call(http.MethodPost, "/api/ingress/deploy", tunnelHTTPAdmin, deployReq{tn.ID, tn.PlanHash}, http.StatusAccepted, &queued)
	f.call(http.MethodPost, "/api/ingress/deploy", tunnelHTTPAdmin, deployReq{tn.ID, tn.PlanHash}, http.StatusAccepted, &queued)
	if queued.State != "queued" || queued.JobID != held.JobID || queued.ReadyToApply || queued.ReceiptVerified || !queued.ReconciliationPending {
		t.Fatalf("queued response overstated: %+v", queued)
	}
	if applyJobs(f.store) != 1 {
		t.Fatalf("expected exactly one apply job, got %d", applyJobs(f.store))
	}
	var status publicIngressOperationResponse
	f.call(http.MethodGet, "/api/ingress/deploy?tunnel_id="+url.QueryEscape(tn.ID), tunnelHTTPAdmin, nil, http.StatusOK, &status)
	if status.State != "queued" || status.ReceiptVerified || status.ReadyToApply || status.OperationID != held.OperationID {
		t.Fatalf("status endpoint overstated: %+v", status)
	}
	f.call(http.MethodGet, "/api/ingress/deploy?tunnel_id="+url.QueryEscape(tn.ID), "", nil, http.StatusUnauthorized, nil)
}

func TestIngressRetireAPIRequiresExactAppliedReceipt(t *testing.T) {
	f := newTunnelHTTPFixture(t)
	tn := glueTunnel(f, 54620)
	f.call(http.MethodPost, "/api/ingress/retire", tunnelHTTPAdmin, deployReq{tn.ID, tn.PlanHash}, http.StatusConflict, nil)
	f.call(http.MethodPost, "/api/ingress/deploy", tunnelHTTPAdmin, deployReq{tn.ID, tn.PlanHash}, http.StatusAccepted, nil)
	// Staged and held (or queued) but no applied receipt: Retire stays refused.
	f.call(http.MethodPost, "/api/ingress/retire", tunnelHTTPAdmin, deployReq{tn.ID, tn.PlanHash}, http.StatusConflict, nil)
	f.pullCaps("ir-1", testIngressCapability)
	f.call(http.MethodPost, "/api/ingress/retire", tunnelHTTPAdmin, deployReq{tn.ID, tn.PlanHash}, http.StatusConflict, nil)
	op, _ := f.store.GetPublicIngressOperation(tn.ID)
	if op.RetireCommand != nil {
		t.Fatal("retire was queued without an applied receipt")
	}
	f.call(http.MethodGet, "/api/ingress/retire", "", nil, http.StatusUnauthorized, nil)
	f.call(http.MethodPut, "/api/ingress/deploy", tunnelHTTPAdmin, nil, http.StatusMethodNotAllowed, nil)
}
