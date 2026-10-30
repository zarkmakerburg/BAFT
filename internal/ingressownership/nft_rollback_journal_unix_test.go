//go:build unix

package ingressownership

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

func TestNFTLabRollbackRetriesDurableIntentAndFinalizationFaults(t *testing.T) {
	for _, tc := range []struct {
		name, point string
		sequence    int
		phase       MutationPhase
		rule        uint64
	}{
		{"before-intent", "before_record", 3, PhaseListenerBound, 42},
		{"after-intent", "after_record", 3, PhaseRetireIntent, 42},
		{"before-final", "before_record", 4, PhaseRetireIntent, 0},
		{"after-final", "after_record", 4, PhaseRetired, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			journal, err := OpenDurableJournal(newJournalTestDir(t))
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			journal.fault = func(point string) error {
				if point == tc.point {
					count++
					if count == tc.sequence {
						return errFinalizeFault
					}
				}
				return nil
			}
			spec := testSpec(t, "rollback-journal")
			owner, err := ingressmanifest.Preview(spec)
			if err != nil {
				t.Fatal(err)
			}
			fake := &nftLabFake{chainHandle: 7, partialAdd: true}
			backend := &NFTLabBackend{Runner: fake, ExpectedChainHandle: 7}
			tx := &NFTLabTransaction{Runtime: NewRuntime(), Firewall: backend, Journal: journal}
			if err := tx.Apply(context.Background(), spec, owner); err == nil {
				t.Fatal("interrupted failed Apply reported success")
			}
			defer tx.Runtime.Retire(spec.Config.Listen, owner)
			current, _, err := journal.Current(owner)
			if err != nil || current.Phase != tc.phase || fake.ruleHandle != tc.rule {
				t.Fatalf("wrong fault boundary: %+v rule=%d err=%v", current, fake.ruleHandle, err)
			}
			journal.fault = nil
			foreign, err := listenWithForensics(t, "tcp4", spec.Config.Listen)
			if err != nil {
				t.Fatal(err)
			}
			defer foreign.Close()
			commands := len(fake.commands)
			restarted := &NFTLabTransaction{Runtime: NewRuntime(), Firewall: backend, Journal: journal}
			if err := restarted.Retire(context.Background(), spec, owner); err == nil {
				t.Fatal("restart adopted rollback record")
			}
			if len(fake.commands) != commands {
				t.Fatal("restart mutated firewall")
			}
			if err := tx.Retire(context.Background(), spec, owner); err != nil {
				t.Fatalf("same-process rollback retry: %v", err)
			}
			current, _, err = journal.Current(owner)
			if err != nil || current.Phase != PhaseRetired || fake.ruleHandle != 0 {
				t.Fatalf("rollback not retired: %+v %v", current, err)
			}
			raw, err := os.ReadFile(filepath.Join(journal.dir, "head.json"))
			var head journalHead
			if err != nil || json.Unmarshal(raw, &head) != nil || head.Sequence != 4 {
				t.Fatalf("rollback head not durable: %s %v", raw, err)
			}
			c, err := net.Dial("tcp4", foreign.Addr().String())
			if err != nil {
				t.Fatalf("rollback closed replacement listener: %v", err)
			}
			_ = c.Close()
		})
	}
}
