package tunnelnode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestMetricsClaimConcurrentExclusiveAndPersistent(t *testing.T) {
	root := newNode(t)
	a, err := root.Manager.ForInstance("alpha", "127.0.0.1:9301")
	if err != nil {
		t.Fatal(err)
	}
	b, err := root.Manager.ForInstance("beta", "127.0.0.1:9302")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, manager := range []*Manager{a, b} {
		wg.Add(1)
		go func(m *Manager) { defer wg.Done(); _, err := m.ClaimMetricsPort(9321); results <- err }(manager)
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one successful exclusive claim, got %d", successes)
	}
	if _, err := a.ClaimMetricsPort(9321); err == nil {
		t.Fatal("persistent claim reused")
	}
}
func TestMetricsClaimRejectsDefaultAndInvalidPort(t *testing.T) {
	root := newNode(t)
	if _, err := root.Manager.ClaimMetricsPort(9321); err == nil {
		t.Fatal("legacy manager acquired claim")
	}
	scoped, err := root.Manager.ForInstance("alpha", "127.0.0.1:9301")
	if err != nil {
		t.Fatal(err)
	}
	for _, port := range []int{0, 65536, -1} {
		if _, err := scoped.ClaimMetricsPort(port); err == nil {
			t.Fatalf("invalid port %d accepted", port)
		}
	}
	path, err := scoped.ClaimMetricsPort(9321)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.TrimSpace(string(data)) != "alpha" {
		t.Fatalf("claim owner mismatch: %q %v", data, err)
	}
}

func TestMetricsClaimRejectsPublicClaimDirectory(t *testing.T) {
	root := newNode(t)
	scoped, err := root.Manager.ForInstance("alpha", "127.0.0.1:9301")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root.Manager.ConfigDir, ".metrics-claims")
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := scoped.ClaimMetricsPort(9321); err == nil {
		t.Fatal("world-readable claims directory accepted")
	}
}

func TestScopedPrepareEXDoesNotTouchExistingInstanceOnClaimConflict(t *testing.T) {
	root := newNode(t)
	scoped, err := root.Manager.ForInstance("alpha", "127.0.0.1:9301")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = scoped.ClaimMetricsPort(9301); err != nil {
		t.Fatal(err)
	}
	_, err = scoped.PrepareEX(context.Background(), "new-pair", ExParams{
		PublicAddress: "127.0.0.1", Port: 8443, Target: "127.0.0.1:1443", RouteID: "r1",
	})
	if err == nil {
		t.Fatal("prepare reused already claimed metrics port")
	}
	if _, statErr := os.Stat(scoped.txnPath("new-pair")); !os.IsNotExist(statErr) {
		t.Fatalf("transaction was created despite port conflict: %v", statErr)
	}
	if root.host.called("stop") || root.host.called("restart") {
		t.Fatal("claim conflict disturbed an existing service")
	}
}

func TestMetricsClaimReleaseRejectsOtherOwner(t *testing.T) {
	root := newNode(t)
	a, err := root.Manager.ForInstance("alpha", "127.0.0.1:9301")
	if err != nil {
		t.Fatal(err)
	}
	b, err := root.Manager.ForInstance("beta", "127.0.0.1:9302")
	if err != nil {
		t.Fatal(err)
	}
	path, err := a.ClaimMetricsPort(9321)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.ReleaseMetricsPort(9321); err == nil {
		t.Fatal("another instance released claim")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("claim disappeared after foreign release:", err)
	}
	if err := a.ReleaseMetricsPort(9321); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("owned claim remains: %v", err)
	}
}

func TestFailedScopedBeginReleasesNewClaim(t *testing.T) {
	root := newNode(t)
	scoped, err := root.Manager.ForInstance("alpha", "127.0.0.1:9301")
	if err != nil {
		t.Fatal(err)
	}
	_, err = scoped.PrepareEX(context.Background(), "../invalid", ExParams{PublicAddress: "127.0.0.1", Port: 8443, Target: "127.0.0.1:1443", RouteID: "r1"})
	if err == nil {
		t.Fatal("unsafe transaction ID accepted")
	}
	path, err := scoped.ClaimMetricsPort(9301)
	if err != nil {
		t.Fatalf("failed begin stranded claim: %v", err)
	}
	if err := scoped.ReleaseMetricsPort(9301); err != nil {
		t.Fatal(err)
	}
	_ = path
}

func TestRollbackFailsClosedOnOrphanedActiveTransaction(t *testing.T) {
	root := newNode(t)
	scoped, err := root.Manager.ForInstance("alpha", "127.0.0.1:9301")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(scoped.activePath()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scoped.activePath(), []byte("pair-orphan\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := scoped.Rollback(context.Background(), "pair-orphan"); err == nil {
		t.Fatal("orphaned active txn falsely reported successful rollback")
	}
	if data, err := os.ReadFile(scoped.activePath()); err != nil || string(data) != "pair-orphan\n" {
		t.Fatalf("orphan marker altered: %q %v", data, err)
	}
}

func TestBeginRefusesOrphanedActiveMarkerFromDifferentChange(t *testing.T) {
	root := newNode(t)
	scoped, err := root.Manager.ForInstance("alpha", "127.0.0.1:9301")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(scoped.activePath()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scoped.activePath(), []byte("orphan-one\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := scoped.begin("new-pair", RoleEX); err == nil {
		t.Fatal("orphaned active marker was ignored")
	}
	if _, err := os.Stat(scoped.stage("new-pair")); !os.IsNotExist(err) {
		t.Fatalf("new transaction touched state: %v", err)
	}
}

func TestBeginRejectsCorruptExistingTransactionWithoutActiveMarker(t *testing.T) {
	root := newNode(t)
	scoped, err := root.Manager.ForInstance("alpha", "127.0.0.1:9301")
	if err != nil {
		t.Fatal(err)
	}
	path := scoped.txnPath("damaged")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{invalid-json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := scoped.begin("damaged", RoleEX); err == nil {
		t.Fatal("corrupt txn was overwritten")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "{invalid-json" {
		t.Fatalf("corrupt state overwritten: %q %v", got, err)
	}
}
