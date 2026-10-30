//go:build unix

package ingressauthority

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

func privateReservationDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "authority")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}
func TestPendingReservationSurvivesRestartAndFencesNumericPort(t *testing.T) {
	dir := privateReservationDir(t)
	store, err := OpenReservationStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := bindingFixture(t)
	if err := store.ReservePending(a); err != nil {
		t.Fatal(err)
	}
	again, err := OpenReservationStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := again.ReservePending(a); err != nil {
		t.Fatalf("idempotent pending claim: %v", err)
	}
	different := a
	different.LeaseID = "other-lease"
	if err := again.ReservePending(different); !errors.Is(err, ErrReserved) {
		t.Fatalf("competing lease accepted: %v", err)
	}
	different = a
	different.ListenEndpoint = "127.0.0.1:1443"
	different.Spec.Config.Listen = different.ListenEndpoint
	proof, err := ingressmanifest.Preview(different.Spec)
	if err != nil {
		t.Fatal(err)
	}
	different.Proof = proof
	if err := again.ReservePending(different); !errors.Is(err, ErrReserved) {
		t.Fatalf("alias claim accepted: %v", err)
	}
}

func TestPendingReservationConcurrentWriters(t *testing.T) {
	dir := privateReservationDir(t)
	const writers = 12
	base := bindingFixture(t)
	result := make(chan error, writers)
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s, err := OpenReservationStore(dir)
			if err != nil {
				result <- err
				return
			}
			b := base
			if n%2 == 0 {
				b.LeaseID = "lease-even"
			} else {
				b.LeaseID = "lease-odd"
			}
			result <- s.ReservePending(b)
		}(i)
	}
	wg.Wait()
	close(result)
	success := 0
	for err := range result {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrReserved) {
			t.Fatal(err)
		}
	}
	if success == 0 {
		t.Fatal("no durable claim established")
	}
}

func TestPendingReservationAcrossProcesses(t *testing.T) {
	dir := os.Getenv("BAFT_AUTHORITY_HELPER_DIR")
	if dir != "" {
		s, err := OpenReservationStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		b := bindingFixture(t)
		b.LeaseID = "child-lease"
		if err := s.ReservePending(b); !errors.Is(err, ErrReserved) {
			t.Fatal(err)
		}
		return
	}
	dir = privateReservationDir(t)
	s, err := OpenReservationStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ReservePending(bindingFixture(t)); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestPendingReservationAcrossProcesses$")
	cmd.Env = append(os.Environ(), "BAFT_AUTHORITY_HELPER_DIR="+dir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("competing child failed: %v %s", err, output)
	}
}

func TestPendingReservationRejectsSecondPortForSameOwner(t *testing.T) {
	dir := privateReservationDir(t)
	s, err := OpenReservationStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	first := bindingFixture(t)
	if err := s.ReservePending(first); err != nil {
		t.Fatal(err)
	}
	other := first
	other.LeaseID = "other-lease"
	other.OperationID = "other-operation"
	other.Spec.Config.Listen = "0.0.0.0:3443"
	other.ListenEndpoint = other.Spec.Config.Listen
	other.Port, other.ConfigPort = 3443, 3443
	other.Proof, err = ingressmanifest.Preview(other.Spec)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ReservePending(other); !errors.Is(err, ErrReserved) {
		t.Fatal(err)
	}
}
