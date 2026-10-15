package tunnelnode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/config"
	"github.com/zarkmakerburg/baft/internal/verification"
)

func verificationPair(t *testing.T, mutate func(*verification.RuntimeReport)) *pair {
	t.Helper()
	p := newPair(t)
	port := freePort(t)
	listen(t, port)
	p.exp.Target = "127.0.0.1:" + strconv.Itoa(port)
	for _, n := range []*node{p.ir, p.ex} {
		n := n
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cfg, err := config.LoadFile(n.liveConfig())
			if err != nil {
				http.NotFound(w, r)
				return
			}
			now := time.Now().UTC()
			report := verification.RuntimeReport{Version: 1, Supported: true, ConfigDigest: verification.ConfigDigest(cfg), BootID: strings.Repeat("1", 32), GeneratedAt: now,
				AuthenticatedPeers: 1, ReceivedBytes: 4096, DeliveredBytes: 4096, LastDeliveredAt: now}
			if mutate != nil {
				mutate(&report)
			}
			json.NewEncoder(w).Encode(report)
		}))
		t.Cleanup(server.Close)
		n.MetricsListen = server.Listener.Addr().String()
	}
	p.build(t, "proof")
	return p
}

func TestDataEvidenceRejectsUnavailableOrUnboundRuntimeReports(t *testing.T) {
	for _, kind := range []string{"version", "config", "boot", "peers", "received-only", "delivered-over-received", "stale-payload", "future-payload", "stale-report", "unsupported"} {
		t.Run(kind, func(t *testing.T) {
			p := verificationPair(t, func(r *verification.RuntimeReport) {
				switch kind {
				case "version":
					r.Version = 0
				case "config":
					r.ConfigDigest = strings.Repeat("f", 64)
				case "boot":
					r.BootID = "not a boot"
				case "peers":
					r.AuthenticatedPeers = 0
				case "received-only":
					r.DeliveredBytes = 0
				case "delivered-over-received":
					r.DeliveredBytes++
				case "stale-payload":
					r.LastDeliveredAt = r.GeneratedAt.Add(-10 * time.Minute)
				case "future-payload":
					r.LastDeliveredAt = r.GeneratedAt.Add(time.Minute)
				case "stale-report":
					r.GeneratedAt = r.GeneratedAt.Add(-time.Minute)
				case "unsupported":
					r.Supported = false
				}
			})
			e, err := p.ir.DataEvidence(context.Background(), "proof")
			if err != nil || e.State != verification.NotAssessed {
				t.Fatalf("invalid runtime report vouches for delivery: %+v %v", e, err)
			}
		})
	}
}

func TestVerifiedFinalizeRequiresSealAndRetainsUnsealedMaterial(t *testing.T) {
	p := verificationPair(t, nil)
	tx, err := p.ir.readTxn("proof")
	if err != nil {
		t.Fatal(err)
	}
	tx.VerificationRequired = true
	if err := p.ir.writeTxn(tx); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(p.ir.liveConfig())
	if _, err := p.ir.Finalize(context.Background(), "proof"); err == nil {
		t.Fatal("unsealed delivery finalized")
	}
	if now, _ := os.ReadFile(p.ir.liveConfig()); string(now) != string(before) || p.ir.activeID() != "proof" {
		t.Fatal("unsealed finalization changed artifacts or released ownership")
	}
	if proof, err := p.ir.Seal(context.Background(), "proof"); err != nil || !proof.ReadyToFinalize {
		t.Fatalf("valid seal failed: %+v %v", proof, err)
	}
	if _, err := p.ir.Finalize(context.Background(), "proof"); err != nil {
		t.Fatal(err)
	}
	if tx, err := p.ir.readTxn("proof"); err != nil || tx.Phase != PhaseFinalized || p.ir.activeID() != "" {
		t.Fatal("verified cleanup did not complete")
	}
}

func TestVerifiedFinalizeRefusesEditsAfterSeal(t *testing.T) {
	p := verificationPair(t, nil)
	if proof, err := p.ex.Seal(context.Background(), "proof"); err != nil || !proof.ReadyToFinalize {
		t.Fatalf("seal: %+v %v", proof, err)
	}
	if err := os.WriteFile(p.ex.unitPath(), []byte("operator edit"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ex.Finalize(context.Background(), "proof"); err == nil {
		t.Fatal("changed artifacts released backups")
	}
	if tx, err := p.ex.readTxn("proof"); err != nil || tx.Phase != PhaseCommitted || p.ex.activeID() != "proof" {
		t.Fatal("refused release changed phase or claim")
	}
}

func TestDataEvidenceRefusesCurrentArtifactDrift(t *testing.T) {
	p := verificationPair(t, nil)
	if err := os.WriteFile(p.ir.markerPath(), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	e, err := p.ir.DataEvidence(context.Background(), "proof")
	if err != nil || e.State != verification.Fail {
		t.Fatalf("artifact drift accepted: %+v %v", e, err)
	}
}

func TestVerifiedFinalizeCleanupFailureRetainsClaimAndRetries(t *testing.T) {
	p := verificationPair(t, nil)
	if _, err := p.ir.Seal(context.Background(), "proof"); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(p.ir.stage("proof"), "psk")
	if err := os.Remove(blocked); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.Mkdir(blocked, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blocked, "operator-file"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ir.Finalize(context.Background(), "proof"); err == nil {
		t.Fatal("cleanup failure was acknowledged")
	}
	tx, err := p.ir.readTxn("proof")
	if err != nil || tx.Phase != PhaseFinalizing || p.ir.activeID() != "proof" {
		t.Fatal("incomplete cleanup released its claim")
	}
	if _, err := p.ir.Rollback(context.Background(), "proof"); err == nil {
		t.Fatal("partial cleanup was treated as rollbackable")
	}
	if err := os.RemoveAll(blocked); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ir.Finalize(context.Background(), "proof"); err != nil {
		t.Fatal(err)
	}
	if p.ir.activeID() != "" {
		t.Fatal("retry did not release the claim")
	}
}

func TestVerifiedSealCannotRenewTheAgeOfPreviouslyDeliveredData(t *testing.T) {
	p := verificationPair(t, nil)
	proof, err := p.ir.Seal(context.Background(), "proof")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := p.ir.readTxn("proof")
	if err != nil {
		t.Fatal(err)
	}
	proof.DataAgeMS = int64((verification.FreshFor - time.Second) / time.Millisecond)
	tx.ReadyProof = &proof
	if err := p.ir.writeTxn(tx); err != nil {
		t.Fatal(err)
	}
	p.ir.Now = func() time.Time { return proof.At.Add(2 * time.Second) }
	if _, err := p.ir.Finalize(context.Background(), "proof"); err == nil {
		t.Fatal("sealing reset the actual data age")
	}
	if p.ir.activeID() != "proof" {
		t.Fatal("expired proof released ownership")
	}
}
