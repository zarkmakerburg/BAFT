//go:build unix

package ingressmanifest

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestUnseenIngressFencePersistsAcrossRestartAndRejectsReplay(t *testing.T) {
	s, dir := fixtureStore(t)
	spec := fixtureSpec()
	_, prepared, err := encode(spec)
	if err != nil {
		t.Fatal(err)
	}
	fence, err := s.FenceUnseen(spec)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Inspect(fence)
	if err != nil || got.Manifest.Phase != PhaseCancelled || got.Manifest.CancelledFromSHA256 != prepared.ManifestSHA256 || got.PreparedBytesMatch || got.LiveOwnershipVerified || got.ReadyToApply {
		t.Fatalf("unseen fence evidence: %+v %v", got, err)
	}
	path := filepath.Join(dir, filename(spec.TunnelID, spec.InstanceID))
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("private fence file: %v %v", info, err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if retry, err := reopened.FenceUnseen(spec); err != nil || retry != fence {
		t.Fatalf("fence retry: %+v %v", retry, err)
	}
	if _, err := reopened.Prepare(spec, nil); err == nil {
		t.Fatal("delayed generation-1 prepare recreated cancelled state")
	}
	spec.Generation = 2
	if _, err := reopened.Prepare(spec, &prepared); err == nil {
		t.Fatal("delayed prepared proof bypassed unseen fence")
	}
	next, err := reopened.Prepare(spec, &fence)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	spec.Generation = 1
	if _, err := s.FenceUnseen(spec); err == nil {
		t.Fatal("stale unseen fence affected newer generation")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("stale fence changed newer bytes: %v", err)
	}
	if current, err := s.Inspect(next); err != nil || !current.PreparedBytesMatch {
		t.Fatalf("new generation evidence: %+v %v", current, err)
	}
}

func TestUnseenIngressFenceRefusesExistingPreparedOrDifferentIntent(t *testing.T) {
	s, dir := fixtureStore(t)
	spec := fixtureSpec()
	if _, err := s.Prepare(spec, nil); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, filename(spec.TunnelID, spec.InstanceID))
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.FenceUnseen(spec); err == nil {
		t.Fatal("unseen fence silently cancelled prepared state")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("unseen fence mutated prepared state: %v", err)
	}
	other := fixtureSpec()
	other.TunnelID = "tun-b"
	if _, err := s.FenceUnseen(other); err != nil {
		t.Fatal(err)
	}
	other.Config.Target = "127.0.0.1:1444"
	if _, err := s.FenceUnseen(other); err == nil {
		t.Fatal("different intent adopted existing tombstone")
	}
	other.Generation = 2
	if _, err := s.FenceUnseen(other); err == nil {
		t.Fatal("unseen fence skipped generation-1 admission")
	}
}

func TestUnseenIngressFenceFaultWindowsRemainRecoverable(t *testing.T) {
	for _, point := range []string{"before-file-sync", "before-rename", "before-directory-sync"} {
		t.Run(point, func(t *testing.T) {
			s, dir := fixtureStore(t)
			spec := fixtureSpec()
			s.fault = func(at string) error {
				if at == point {
					return errors.New("injected unseen fence failure")
				}
				return nil
			}
			result, err := s.FenceUnseen(spec)
			if err == nil {
				t.Fatal("fault reported successful unseen fence")
			}
			s.fault = nil
			if point == "before-directory-sync" {
				if !errors.Is(err, ErrDurabilityUncertain) || result.Generation != 1 {
					t.Fatalf("uncertain fence result: %+v %v", result, err)
				}
				if _, err := s.Prepare(spec, nil); err == nil {
					t.Fatal("post-rename fence lost replay protection")
				}
			} else {
				if _, err := os.Stat(filepath.Join(dir, filename(spec.TunnelID, spec.InstanceID))); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("pre-rename failure created manifest: %v", err)
				}
			}
			retry, err := s.FenceUnseen(spec)
			if err != nil || (point == "before-directory-sync" && retry != result) {
				t.Fatalf("failed to retry unseen fence: %+v %v", retry, err)
			}
			if _, err := s.Prepare(spec, nil); err == nil {
				t.Fatal("retried fence allowed delayed prepare")
			}
		})
	}
}

func TestManifestCancelRetainsHighWaterAndBlocksReplay(t *testing.T) {
	s, dir := fixtureStore(t)
	spec := fixtureSpec()
	prepared, err := s.Prepare(spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := s.Cancel(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Generation != prepared.Generation || cancelled.ConfigSHA256 != prepared.ConfigSHA256 || cancelled.ManifestSHA256 == prepared.ManifestSHA256 {
		t.Fatalf("incorrect cancellation high-water: %+v", cancelled)
	}
	inspected, err := s.Inspect(cancelled)
	if err != nil || inspected.Manifest.Phase != PhaseCancelled || !inspected.ManifestBytesMatch || inspected.PreparedBytesMatch || inspected.ReadyToApply || inspected.LiveOwnershipVerified || inspected.Manifest.CancelledFromSHA256 != prepared.ManifestSHA256 {
		t.Fatalf("cancelled evidence: %+v %v", inspected, err)
	}
	for _, proof := range []Proof{prepared, cancelled} {
		again, err := s.Cancel(proof)
		if err != nil || again != cancelled {
			t.Fatalf("cancellation retry: %+v %v", again, err)
		}
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err := reopened.Prepare(spec, nil); err == nil {
		t.Fatal("stale generation1 recreated cancelled state after restart")
	}
	if _, err := reopened.Prepare(spec, &cancelled); err == nil {
		t.Fatal("same-generation prepare reused cancelled generation")
	}
	if _, err := reopened.Inspect(prepared); err == nil {
		t.Fatal("old prepared proof remained current")
	}
	spec.Generation = 2
	if _, err := reopened.Prepare(spec, &prepared); err == nil {
		t.Fatal("pre-cancellation proof advanced cancelled state")
	}
	next, err := reopened.Prepare(spec, &cancelled)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(dir, filename(spec.TunnelID, spec.InstanceID)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Cancel(prepared); err == nil {
		t.Fatal("stale cancellation affected newer generation")
	}
	if _, err := s.Cancel(cancelled); err == nil {
		t.Fatal("old tombstone cancelled newer generation")
	}
	after, _ := os.ReadFile(filepath.Join(dir, filename(spec.TunnelID, spec.InstanceID)))
	if !bytes.Equal(before, after) {
		t.Fatal("stale cancel changed newer bytes")
	}
	current, err := s.Inspect(next)
	if err != nil || current.Manifest.Phase != PhasePrepared || !current.PreparedBytesMatch || !current.ManifestBytesMatch {
		t.Fatalf("new generation: %+v %v", current, err)
	}
}

func TestManifestCancelFaultWindowsAndRetry(t *testing.T) {
	for _, point := range []string{"before-file-sync", "before-rename", "before-directory-sync"} {
		t.Run(point, func(t *testing.T) {
			s, dir := fixtureStore(t)
			spec := fixtureSpec()
			prepared, err := s.Prepare(spec, nil)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, filename(spec.TunnelID, spec.InstanceID))
			before, _ := os.ReadFile(path)
			s.fault = func(at string) error {
				if at == point {
					return errors.New("injected cancellation failure")
				}
				return nil
			}
			result, err := s.Cancel(prepared)
			if err == nil {
				t.Fatal("cancellation fault reported success")
			}
			s.fault = nil
			if point == "before-directory-sync" {
				if !errors.Is(err, ErrDurabilityUncertain) || result.Generation != prepared.Generation {
					t.Fatalf("post-rename uncertainty: %+v %v", result, err)
				}
				if _, err := s.Prepare(spec, nil); err == nil {
					t.Fatal("uncertain cancellation lost replay fence")
				}
				current, err := s.Inspect(result)
				if err != nil || current.Manifest.Phase != PhaseCancelled {
					t.Fatalf("reconcile cancellation: %+v %v", current, err)
				}
				if retried, err := s.Cancel(prepared); err != nil || retried != result {
					t.Fatalf("retry uncertain cancellation: %+v %v", retried, err)
				}
			} else {
				after, _ := os.ReadFile(path)
				if !bytes.Equal(before, after) {
					t.Fatal("pre-rename cancellation changed prepared bytes")
				}
				current, err := s.Inspect(prepared)
				if err != nil || current.Manifest.Phase != PhasePrepared {
					t.Fatalf("prior manifest lost: %+v %v", current, err)
				}
				if _, err := s.Cancel(prepared); err != nil {
					t.Fatalf("retry failed cancellation: %v", err)
				}
			}
		})
	}
}

func TestManifestCancelReadsLegacyPreparedSchema(t *testing.T) {
	s, dir := fixtureStore(t)
	spec := fixtureSpec()
	if _, err := s.Prepare(spec, nil); err != nil {
		t.Fatal(err)
	}
	legacy, proof, err := encodeRecord(Manifest{SchemaVersion: 1, Phase: PhasePrepared, Spec: spec})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, filename(spec.TunnelID, spec.InstanceID))
	if err := os.WriteFile(path, legacy, 0600); err != nil {
		t.Fatal(err)
	}
	current, err := s.Inspect(proof)
	if err != nil || current.Manifest.SchemaVersion != 1 || !current.PreparedBytesMatch {
		t.Fatalf("legacy prepared read: %+v %v", current, err)
	}
	retry, err := s.Prepare(spec, nil)
	if err != nil || retry != proof {
		t.Fatalf("legacy idempotent prepare: %+v %v", retry, err)
	}
	raw, _ := os.ReadFile(path)
	if !bytes.Equal(raw, legacy) {
		t.Fatal("legacy idempotence unexpectedly rewrote bytes")
	}
	cancelled, err := s.Cancel(proof)
	if err != nil {
		t.Fatal(err)
	}
	current, err = s.Inspect(cancelled)
	if err != nil || current.Manifest.SchemaVersion != 2 || current.Manifest.Phase != PhaseCancelled {
		t.Fatalf("legacy cancellation upgrade: %+v %v", current, err)
	}
	if _, err := s.Prepare(spec, nil); err == nil {
		t.Fatal("legacy generation1 replay bypassed tombstone")
	}
	if _, _, err := encodeRecord(Manifest{SchemaVersion: 1, Phase: PhaseCancelled, Spec: spec, CancelledFromSHA256: proof.ManifestSHA256}); err == nil {
		t.Fatal("schema1 cannot represent cancellation")
	}
}

func TestManifestCancelAndAdvanceHaveOneWinner(t *testing.T) {
	s, dir := fixtureStore(t)
	spec := fixtureSpec()
	first, err := s.Prepare(spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	other, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Go(func() { <-start; _, err := s.Cancel(first); results <- err })
	wg.Go(func() {
		<-start
		next := spec
		next.Generation = 2
		_, err := other.Prepare(next, &first)
		results <- err
	})
	close(start)
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("competing cancel and advance got %d successes", success)
	}
	raw, err := os.ReadFile(filepath.Join(dir, filename(spec.TunnelID, spec.InstanceID)))
	if err != nil {
		t.Fatal(err)
	}
	current, _, err := decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !((current.Phase == PhaseCancelled && current.Spec.Generation == 1) || (current.Phase == PhasePrepared && current.Spec.Generation == 2)) {
		t.Fatalf("non-monotonic final state: %+v", current)
	}
	if _, err := s.Prepare(spec, nil); err == nil {
		t.Fatal("generation1 replay succeeded after competing transition")
	}
}

func TestManifestCancelRefusesWrongProofAndClosedStore(t *testing.T) {
	s, dir := fixtureStore(t)
	spec := fixtureSpec()
	prepared, err := s.Prepare(spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	wrong := prepared
	wrong.Generation++
	if _, err := s.Cancel(wrong); err == nil {
		t.Fatal("incorrect generation cancelled resource")
	}
	wrong = prepared
	wrong.ConfigSHA256 = string(bytes.Repeat([]byte{'0'}, 64))
	if _, err := s.Cancel(wrong); err == nil {
		t.Fatal("incorrect config digest cancelled resource")
	}
	wrong = prepared
	wrong.TunnelID = "other"
	if _, err := s.Cancel(wrong); !errors.Is(err, ErrNotPrepared) {
		t.Fatalf("different owner cancellation: %v", err)
	}
	if got, err := s.Inspect(prepared); err != nil || got.Manifest.Phase != PhasePrepared {
		t.Fatalf("rejected cancel changed resource: %+v %v", got, err)
	}
	info, err := os.Stat(filepath.Join(dir, filename(spec.TunnelID, spec.InstanceID)))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("manifest permissions: %v %v", info, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Cancel(prepared); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("cancel after close: %v", err)
	}
}
