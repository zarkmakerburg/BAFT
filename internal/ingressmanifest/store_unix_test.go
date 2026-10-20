//go:build unix

package ingressmanifest

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func fixtureSpec() Spec {
	return Spec{TunnelID: "tun-a", InstanceID: "ir-a", Generation: 1, Config: Config{Listen: "0.0.0.0:54619", Target: "127.0.0.1:1443"}}
}
func fixtureStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, dir
}

func TestPreparedManifestRoundTripAndGeneration(t *testing.T) {
	s, dir := fixtureStore(t)
	spec := fixtureSpec()
	first, err := s.Prepare(spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := s.Inspect(first)
	if err != nil || !inspection.PreparedBytesMatch || inspection.ReadyToApply || inspection.LiveOwnershipVerified {
		t.Fatalf("prepared byte evidence: %+v %v", inspection, err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, filename(spec.TunnelID, spec.InstanceID)))
	if err != nil {
		t.Fatal(err)
	}
	if hash(raw) != first.ManifestSHA256 {
		t.Fatal("manifest digest is not the actual file digest")
	}
	for _, name := range []string{".lock", filename(spec.TunnelID, spec.InstanceID)} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("private %s mode: %v %v", name, info, err)
		}
	}
	same, err := s.Prepare(spec, nil)
	if err != nil || same != first {
		t.Fatalf("idempotent retry: %+v %v", same, err)
	}
	spec.Generation = 2
	spec.Config.MaxClients = 64
	if _, err := s.Prepare(spec, nil); err == nil {
		t.Fatal("generation advance without previous proof accepted")
	}
	stale := first
	stale.ConfigSHA256 = strings.Repeat("0", 64)
	if _, err := s.Prepare(spec, &stale); err == nil {
		t.Fatal("wrong previous digest accepted")
	}
	second, err := s.Prepare(spec, &first)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Inspect(first); err == nil {
		t.Fatal("stale generation proof accepted")
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got, err := reopened.Inspect(second); err != nil || got.Proof != second {
		t.Fatalf("restart inspection: %+v %v", got, err)
	}
	spec.Generation = 1
	if _, err := s.Prepare(spec, &second); err == nil {
		t.Fatal("generation rollback accepted")
	}
	spec.Generation = 4
	if _, err := s.Prepare(spec, &second); err == nil {
		t.Fatal("generation skip accepted")
	}
	wrongOwner := second
	wrongOwner.TunnelID = "other"
	if _, err := s.Inspect(wrongOwner); !errors.Is(err, ErrNotPrepared) {
		t.Fatalf("cross-owner proof: %v", err)
	}
	wrongDigest := second
	wrongDigest.ManifestSHA256 = strings.Repeat("0", 64)
	if _, err := s.Inspect(wrongDigest); err == nil {
		t.Fatal("wrong manifest digest accepted")
	}
}

func TestPreparedManifestInspectionDoesNotCreateAnything(t *testing.T) {
	s, dir := fixtureStore(t)
	_, expected, err := encode(fixtureSpec())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Inspect(expected); !errors.Is(err, ErrNotPrepared) {
		t.Fatalf("unprepared inspection: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("inspect mutated filesystem: %+v %v", entries, err)
	}
}

func TestPreparedManifestFaultsPreservePriorBytes(t *testing.T) {
	for _, point := range []string{"before-file-sync", "before-rename", "before-directory-sync"} {
		t.Run(point, func(t *testing.T) {
			s, dir := fixtureStore(t)
			spec := fixtureSpec()
			first, err := s.Prepare(spec, nil)
			if err != nil {
				t.Fatal(err)
			}
			old, err := os.ReadFile(filepath.Join(dir, filename(spec.TunnelID, spec.InstanceID)))
			if err != nil {
				t.Fatal(err)
			}
			spec.Generation = 2
			spec.Config.MaxClients = 32
			s.fault = func(at string) error {
				if at == point {
					return errors.New("injected failure")
				}
				return nil
			}
			result, err := s.Prepare(spec, &first)
			if err == nil {
				t.Fatal("fault reported success")
			}
			entries, _ := os.ReadDir(dir)
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".stage-") {
					t.Fatal("failed prepare left staging files")
				}
			}
			s.fault = nil
			if point == "before-directory-sync" {
				if !errors.Is(err, ErrDurabilityUncertain) || result.Generation != 2 {
					t.Fatalf("rename uncertainty not explicit: %+v %v", result, err)
				}
				if _, err := s.Inspect(result); err != nil {
					t.Fatalf("renamed bytes cannot be reconciled: %v", err)
				}
				if retried, err := s.Prepare(spec, &first); err != nil || retried != result {
					t.Fatalf("durability retry: %+v %v", retried, err)
				}
			} else {
				now, _ := os.ReadFile(filepath.Join(dir, filename(spec.TunnelID, spec.InstanceID)))
				if !bytes.Equal(old, now) {
					t.Fatal("pre-rename failure changed existing manifest")
				}
				if _, err := s.Inspect(first); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestPreparedManifestRejectsTamperingAndUnsafeFiles(t *testing.T) {
	for _, attack := range []string{"changed", "trailing", "duplicate", "unknown", "public-mode", "symlink", "hardlink", "fifo"} {
		t.Run(attack, func(t *testing.T) {
			s, dir := fixtureStore(t)
			spec := fixtureSpec()
			proof, err := s.Prepare(spec, nil)
			if err != nil {
				t.Fatal(err)
			}
			name := filepath.Join(dir, filename(spec.TunnelID, spec.InstanceID))
			raw, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			switch attack {
			case "changed":
				raw = bytes.Replace(raw, []byte("54619"), []byte("54620"), 1)
				err = os.WriteFile(name, raw, 0600)
			case "trailing":
				err = os.WriteFile(name, append(raw, []byte(` {}`)...), 0600)
			case "duplicate":
				err = os.WriteFile(name, append(raw[:len(raw)-1], []byte(`,"phase":"PREPARED"}`)...), 0600)
			case "unknown":
				err = os.WriteFile(name, append(raw[:len(raw)-1], []byte(`,"ownership_verified":true}`)...), 0600)
			case "public-mode":
				err = os.Chmod(name, 0644)
			case "symlink":
				if err = os.Remove(name); err == nil {
					err = os.Symlink(".lock", name)
				}
			case "hardlink":
				err = os.Link(name, name+"-duplicate")
			case "fifo":
				if err = os.Remove(name); err == nil {
					err = syscall.Mkfifo(name, 0600)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.Inspect(proof); err == nil {
				t.Fatal("unsafe or changed file accepted")
			}
			if _, err := s.Prepare(spec, nil); err == nil {
				t.Fatal("prepare adopted unsafe or changed resource")
			}
		})
	}
}

func TestPreparedManifestDirectoryAndInputValidation(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if s, err := Open(dir); err == nil {
		s.Close()
		t.Fatal("public directory accepted")
	}
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if s, err := Open(link); err == nil {
		s.Close()
		t.Fatal("symlink directory accepted")
	}
	s, private := fixtureStore(t)
	for _, mutate := range []func(*Spec){
		func(s *Spec) { s.TunnelID = "../escape" }, func(s *Spec) { s.InstanceID = "default" }, func(s *Spec) { s.Generation = 0 },
		func(s *Spec) { s.InstanceID = "../escape" },
		func(s *Spec) { s.Config.Listen = "0.0.0.0:1023" }, func(s *Spec) { s.Config.Target = "127.0.0.1:22" },
		func(s *Spec) { s.Config.Target = "localhost:1443" }, func(s *Spec) { s.Config.MaxClients = 1025 }, func(s *Spec) { s.Config.DialTimeoutMS = 10001 },
	} {
		spec := fixtureSpec()
		mutate(&spec)
		if _, err := s.Prepare(spec, nil); err == nil {
			t.Fatalf("invalid spec accepted: %+v", spec)
		}
	}
	entries, _ := os.ReadDir(private)
	if len(entries) != 0 {
		t.Fatal("invalid requests created state")
	}
}

func TestPreparedManifestConcurrentStoresHaveOneWriter(t *testing.T) {
	first, dir := fixtureStore(t)
	second, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	start := make(chan struct{})
	results := make(chan error, 2)
	var writers sync.WaitGroup
	for i, store := range []*Store{first, second} {
		writers.Go(func() {
			<-start
			spec := fixtureSpec()
			spec.Config.MaxClients = 64 + i
			_, err := store.Prepare(spec, nil)
			results <- err
		})
	}
	close(start)
	writers.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("conflicting writers got %d successes", successes)
	}
	raw, err := os.ReadFile(filepath.Join(dir, filename("tun-a", "ir-a")))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := decode(raw); err != nil {
		t.Fatalf("concurrent write left invalid bytes: %v", err)
	}
}

func TestManifestProcessLockHelper(t *testing.T) {
	dir := os.Getenv("BAFT_TEST_INGRESS_MANIFEST_LOCK_DIR")
	if dir == "" {
		return
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	unlock, err := s.lock(true)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	fmt.Fprintln(os.Stdout, "manifest-lock-held")
	_, _ = io.Copy(io.Discard, os.Stdin)
}

func TestPreparedManifestLockIsCrossProcess(t *testing.T) {
	s, dir := fixtureStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestManifestProcessLockHelper$")
	cmd.Env = append(os.Environ(), "BAFT_TEST_INGRESS_MANIFEST_LOCK_DIR="+dir)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { stdin.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() || scanner.Text() != "manifest-lock-held" {
		t.Fatalf("lock child failed: %s", stderr.String())
	}
	if _, err := s.Prepare(fixtureSpec(), nil); !errors.Is(err, ErrBusy) {
		t.Fatalf("cross-process lock bypassed: %v", err)
	}
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("lock child exit: %v %s", err, stderr.String())
	}
	if _, err := s.Prepare(fixtureSpec(), nil); err != nil {
		t.Fatalf("released process lock stayed held: %v", err)
	}
}

func TestPreparedManifestClosedStoreIsSafe(t *testing.T) {
	s, _ := fixtureStore(t)
	proof, err := s.Prepare(fixtureSpec(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("double close: %v", err)
	}
	if _, err := s.Prepare(fixtureSpec(), nil); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("prepare after close: %v", err)
	}
	if _, err := s.Inspect(proof); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("inspect after close: %v", err)
	}
	var nilStore *Store
	if err := nilStore.Close(); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("nil close: %v", err)
	}
	var zero Store
	if _, err := zero.Prepare(fixtureSpec(), nil); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("unopened prepare: %v", err)
	}
}

func TestPreparedManifestRejectsForeignOwnerAtResourcePath(t *testing.T) {
	s, dir := fixtureStore(t)
	desired := fixtureSpec()
	foreign := desired
	foreign.TunnelID = "foreign-tunnel"
	raw, foreignProof, err := encode(foreign)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, filename(desired.TunnelID, desired.InstanceID))
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	desired.Generation = 2
	if _, err := s.Prepare(desired, &foreignProof); err == nil {
		t.Fatal("foreign ownership marker was adopted")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, raw) {
		t.Fatal("rejected foreign resource was changed")
	}
}
