//go:build unix

package ingressownership

import (
	"context"
	"errors"
	"net"
	"strconv"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

func TestJournaledLabTransactionPersistsBeforeMutation(t *testing.T) {
	j, err := OpenDurableJournal(newJournalTestDir(t))
	if err != nil {
		t.Fatal(err)
	}
	tx := &LabTransaction{Runtime: NewRuntime(), Firewall: NewMemoryFirewall(), Journal: j}
	spec := testSpec(t, "journaled-tx")
	owner, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	j.fault = func(point string) error {
		if point == "before_record" {
			return errors.New("injected intent failure")
		}
		return nil
	}
	if err := tx.Apply(context.Background(), spec, owner); err == nil {
		t.Fatal("mutated after intent persistence failure")
	}
	ln, err := net.Listen("tcp", spec.Config.Listen)
	if err != nil {
		t.Fatalf("listener appeared before durable intent: %v", err)
	}
	ln.Close()
	got, err := j.Recover()
	if err != nil || len(got.Records) != 0 {
		t.Fatalf("failed intent entered journal: %+v %v", got, err)
	}
	j.fault = nil
	if err := tx.Apply(context.Background(), spec, owner); err != nil {
		t.Fatal(err)
	}
	if err := tx.Apply(context.Background(), spec, owner); err != nil {
		t.Fatalf("verified replay: %v", err)
	}
	got, err = j.Recover()
	if err != nil || len(got.NeedsReconciliation) != 1 || got.NeedsReconciliation[0].Phase != PhaseVerified {
		t.Fatalf("active record was adopted or lost: %+v %v", got, err)
	}
	j.fault = func(point string) error {
		if point == "before_record" {
			return errors.New("injected retire-intent failure")
		}
		return nil
	}
	if err := tx.Retire(spec.Config.Listen, owner); err == nil {
		t.Fatal("retired without durable retire intent")
	}
	if err := tx.Apply(context.Background(), spec, owner); err != nil {
		t.Fatalf("failed retire intent disrupted live owner: %v", err)
	}
	j.fault = nil
	if err := tx.Retire(spec.Config.Listen, owner); err != nil {
		t.Fatal(err)
	}
	got, err = j.Recover()
	if err != nil || len(got.NeedsReconciliation) != 0 || len(got.Records) != 1 || got.Records[0].Phase != PhaseRetired {
		t.Fatalf("retired record: %+v %v", got, err)
	}
}

func TestJournaledLabTransactionFailureLeavesUnverifiedIntent(t *testing.T) {
	j, err := OpenDurableJournal(newJournalTestDir(t))
	if err != nil {
		t.Fatal(err)
	}
	tx := &LabTransaction{Runtime: NewRuntime(), Firewall: NewMemoryFirewall(), Journal: j}
	spec := testSpec(t, "journaled-conflict")
	owner, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", spec.Config.Listen)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if err := tx.Apply(context.Background(), spec, owner); err == nil {
		t.Fatal("accepted unrelated listener conflict")
	}
	record, found, err := j.Current(owner)
	if err != nil || !found || record.Phase != PhaseIntent {
		t.Fatalf("lost uncertain intent: %+v %t %v", record, found, err)
	}
	_, port, _ := net.SplitHostPort(spec.Config.Listen)
	n, _ := strconv.Atoi(port)
	if ok, err := tx.Firewall.Verify(uint16(n), owner); err != nil || ok {
		t.Fatalf("firewall changed after bind conflict: %t %v", ok, err)
	}
}
