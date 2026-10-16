package tunnelnode

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

type rollbackFailureSystem struct {
	System
	verb string
	hook func()
}

func (s *rollbackFailureSystem) Systemctl(ctx context.Context, args ...string) (string, error) {
	if args[0] == s.verb {
		return "", errors.New("injected rollback " + s.verb + " failure")
	}
	out, err := s.System.Systemctl(ctx, args...)
	if s.hook != nil && args[0] == "stop" {
		s.hook()
		s.hook = nil
	}
	return out, err
}

func TestRollbackReportsServiceFailuresAndRemainsRetryable(t *testing.T) {
	for _, verb := range []string{"daemon-reload", "stop", "disable"} {
		t.Run(verb, func(t *testing.T) {
			p := newPair(t)
			p.build(t, "rollback-errors")
			base := p.ex.System
			p.ex.System = &rollbackFailureSystem{System: base, verb: verb}
			msg, err := p.ex.Rollback(context.Background(), "rollback-errors")
			if err == nil || !strings.Contains(msg, "incomplete") {
				t.Fatalf("rollback error hidden: %q %v", msg, err)
			}
			if tx, e := p.ex.readTxn("rollback-errors"); e != nil || tx.Phase == PhaseRolledBack {
				t.Fatalf("incomplete rollback recorded as complete: %+v %v", tx, e)
			}
			p.ex.System = base
			if msg, err := p.ex.Rollback(context.Background(), "rollback-errors"); err != nil || msg != "rolled back" {
				t.Fatalf("repair could not resume rollback: %q %v", msg, err)
			}
		})
	}
}

func TestRollbackRefusesCorruptTransactionInsteadOfReportingNothing(t *testing.T) {
	p := newPair(t)
	p.build(t, "corrupt-rollback")
	if err := os.WriteFile(p.ex.txnPath("corrupt-rollback"), []byte("invalid transaction"), 0600); err != nil {
		t.Fatal(err)
	}
	if msg, err := p.ex.Rollback(context.Background(), "corrupt-rollback"); err == nil {
		t.Fatalf("corrupt transaction falsely acknowledged: %q", msg)
	}
	if !p.ex.host.active {
		t.Fatal("corrupt transaction changed the service")
	}
}

func TestRollbackDoesNotAcknowledgeFailedCompletionReceipt(t *testing.T) {
	p := newPair(t)
	p.build(t, "receipt-error")
	p.ex.System = &rollbackFailureSystem{System: p.ex.System, hook: func() {
		path := p.ex.txnPath("receipt-error")
		if err := os.Rename(path, path+".saved"); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}}
	if msg, err := p.ex.Rollback(context.Background(), "receipt-error"); err == nil {
		t.Fatalf("receipt failure falsely acknowledged: %q", msg)
	}
	if p.ex.activeID() != "receipt-error" {
		t.Fatal("failed receipt released the active change")
	}
}

func TestRollbackRefusesMissingTransactionWithLiveOwnership(t *testing.T) {
	for _, clearReceipt := range []bool{false, true} {
		p := newPair(t)
		p.build(t, "missing-rollback")
		if err := os.Remove(p.ex.txnPath("missing-rollback")); err != nil {
			t.Fatal(err)
		}
		if clearReceipt {
			p.ex.clearActive()
		}
		if msg, err := p.ex.Rollback(context.Background(), "missing-rollback"); err == nil {
			t.Fatalf("lost transaction falsely acknowledged: %q", msg)
		}
		if !p.ex.host.active {
			t.Fatal("missing transaction changed service state")
		}
	}
}

func TestRollbackRestoresActiveButDisabledService(t *testing.T) {
	p := newPair(t)
	p.build(t, "first-disabled")
	for _, m := range []*Manager{p.ir.Manager, p.ex.Manager} {
		if _, err := m.Finalize(context.Background(), "first-disabled"); err != nil {
			t.Fatal(err)
		}
	}
	// systemctl start can leave a service active but disabled at boot.
	p.ex.host.enabled = false
	p.exp.Port = freePort(t)
	p.build(t, "edit-disabled")
	if _, err := p.ex.Rollback(context.Background(), "edit-disabled"); err != nil {
		t.Fatal(err)
	}
	if !p.ex.host.active || p.ex.host.enabled {
		t.Fatal("rollback did not restore the previous active/disabled state")
	}
}
