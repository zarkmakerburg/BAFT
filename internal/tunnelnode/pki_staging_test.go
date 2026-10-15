package tunnelnode

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func finalizedPair(t *testing.T) *pair {
	t.Helper()
	p := newPair(t)
	p.build(t, "old-host")
	for _, n := range []*node{p.ex, p.ir} {
		if _, err := n.Finalize(context.Background(), "old-host"); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func TestPKIPrepareKeepsActiveMaterial(t *testing.T) {
	p := finalizedPair(t)
	old, err := snapshotPKI(p.ex.pkiDir())
	if err != nil {
		t.Fatal(err)
	}
	artifacts := map[string]string{}
	for _, path := range []string{p.ex.liveConfig(), p.ex.unitPath(), p.ex.markerPath(), p.ex.generationPath(), p.ex.noiseKey()} {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		artifacts[path] = string(b)
	}
	calls := len(p.ex.host.calls)
	p.exp.PublicAddress = "127.0.0.2"
	if _, err := p.ex.PrepareEX(context.Background(), "staged", p.exp); err != nil {
		t.Fatal(err)
	}
	live, err := snapshotPKI(p.ex.pkiDir())
	if err != nil || !reflect.DeepEqual(old, live) {
		t.Fatal("prepare changed the certificate tree used by the active service")
	}
	if len(p.ex.host.calls) != calls || !p.ex.host.active {
		t.Fatal("prepare changed the active service")
	}
	for path, want := range artifacts {
		if b, err := os.ReadFile(path); err != nil || string(b) != want {
			t.Fatal("prepare changed a live configuration artifact")
		}
	}
	if _, err := os.Lstat(p.ex.pkiDir() + ".before-staged"); !os.IsNotExist(err) {
		t.Fatal("prepare moved live certificates to the rollback path")
	}
	if _, err := p.ex.Rollback(context.Background(), "staged"); err != nil {
		t.Fatal(err)
	}
	live, err = snapshotPKI(p.ex.pkiDir())
	if err != nil || !reflect.DeepEqual(old, live) || len(p.ex.host.calls) != calls {
		t.Fatal("cancelled preparation changed live material or restarted the service")
	}
}

func TestPKIExplicitRollbackAfterInterruptedActivation(t *testing.T) {
	for _, point := range []string{"tunnel-commit:after-pki-exchange", "tunnel-commit:after-pki-activation", "initial-activation"} {
		t.Run(point, func(t *testing.T) {
			p := newPair(t)
			if point != "initial-activation" {
				p = finalizedPair(t)
				p.exp.PublicAddress = "127.0.0.2"
			}
			old, _ := snapshotPKI(p.ex.pkiDir())
			code, err := p.ex.PrepareEX(context.Background(), "interrupted", p.exp)
			if err != nil {
				t.Fatal(err)
			}
			reply, err := p.ir.PrepareIR(context.Background(), "interrupted", code, p.irp)
			if err != nil {
				t.Fatal(err)
			}
			fault := point
			if point == "initial-activation" {
				fault = "tunnel-commit:after-pki-activation"
			}
			p.ex.Fault = func(at string) error {
				if at == fault {
					return errors.New("injected cutover interruption")
				}
				return nil
			}
			calls := len(p.ex.host.calls)
			if _, err := p.ex.CommitEX(context.Background(), "interrupted", reply); err == nil {
				t.Fatal("interrupted activation acknowledged")
			}
			if p.ex.activeID() != "interrupted" {
				t.Fatal("interrupted activation lost ownership")
			}
			for _, call := range p.ex.host.calls[calls:] {
				if !strings.HasPrefix(call, "is-active ") && !strings.HasPrefix(call, "is-enabled ") {
					t.Fatalf("interrupted activation changed the service: %s", call)
				}
			}
			calls = len(p.ex.host.calls)
			m, err := New(p.ex.Env)
			if err != nil {
				t.Fatal(err)
			}
			m.Fault = nil
			if _, err := m.Rollback(context.Background(), "interrupted"); err != nil {
				t.Fatal(err)
			}
			live, _ := snapshotPKI(m.pkiDir())
			if !reflect.DeepEqual(old, live) || len(p.ex.host.calls) != calls {
				t.Fatal("explicit rollback did not restore the pre-cutover tree without service mutation")
			}
			for _, path := range []string{m.pkiNext("interrupted"), m.pkiDir() + ".before-interrupted"} {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatal("explicit rollback left generated private keys")
				}
			}
		})
	}
}

type corruptPreparedPKISystem struct {
	System
	mode string
}

func (s corruptPreparedPKISystem) Run(ctx context.Context, name string, args ...string) (string, error) {
	out, err := s.System.Run(ctx, name, args...)
	if err != nil || len(args) == 0 || args[0] != "pki" {
		return out, err
	}
	dir := args[2] // fixed pki --dir PATH --host HOST invocation
	if s.mode == "wrong-host" {
		return s.System.Run(ctx, name, "pki", "--dir", dir, "--host", "unrelated.example")
	}
	return out, os.WriteFile(filepath.Join(dir, "server.key"), []byte("invalid generated key"), 0600)
}

func TestPKIPrepareRejectsInvalidGeneratedCertificateBeforeCutover(t *testing.T) {
	for _, mode := range []string{"wrong-host", "bad-key"} {
		t.Run(mode, func(t *testing.T) {
			p := finalizedPair(t)
			old, _ := snapshotPKI(p.ex.pkiDir())
			p.exp.PublicAddress = "127.0.0.2"
			p.ex.System = corruptPreparedPKISystem{System: p.ex.System, mode: mode}
			if _, err := p.ex.PrepareEX(context.Background(), "invalid", p.exp); err == nil {
				t.Fatal("invalid certificate/key was accepted")
			}
			live, _ := snapshotPKI(p.ex.pkiDir())
			if !reflect.DeepEqual(old, live) || p.ex.activeID() != "" {
				t.Fatal("invalid generated material changed live certificates or failed safe cancellation")
			}
		})
	}
}

func TestPKIFinalizeRetainsBackupsOnCertificateDrift(t *testing.T) {
	p, _ := changedHostPair(t)
	if err := os.WriteFile(filepath.Join(p.ex.pkiDir(), "server.pem"), []byte("operator edit"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ex.Finalize(context.Background(), "new-host"); err == nil {
		t.Fatal("finalize accepted changed installed certificates")
	}
	if _, err := os.Stat(p.ex.pkiDir() + ".before-new-host"); err != nil || p.ex.activeID() != "new-host" {
		t.Fatal("refused finalization discarded rollback material or released ownership")
	}
}

func TestPKIRollbackRetainsOlderCommittedReceiptContract(t *testing.T) {
	p, old := changedHostPair(t)
	tx, err := p.ex.readTxn("new-host")
	if err != nil {
		t.Fatal(err)
	}
	// Older committed receipts have the same verified live/backup identities,
	// but none of the new preparation fields. They must still roll back safely.
	tx.PKIHost, tx.PKIStaged = "", false
	if err := p.ex.writeTxn(tx); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ex.Rollback(context.Background(), "new-host"); err != nil {
		t.Fatal(err)
	}
	if !sameDigests(pkiDigests(p.ex.pkiDir()), old) {
		t.Fatal("older committed receipt failed to restore verified certificates")
	}
}

func TestPKIPrepareInitialDoesNotInstallLiveMaterial(t *testing.T) {
	p := newPair(t)
	if _, err := p.ex.PrepareEX(context.Background(), "first-stage", p.exp); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(p.ex.pkiDir()); !os.IsNotExist(err) {
		t.Fatal("prepare installed the new certificate tree before commit")
	}
	if _, err := p.ex.Rollback(context.Background(), "first-stage"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(p.ex.pkiDir() + ".next-first-stage"); !os.IsNotExist(err) {
		t.Fatal("cancelled first preparation retained generated private keys")
	}
}

func TestPKICommitRefusesChangedPreparedOrLiveMaterial(t *testing.T) {
	for _, which := range []string{"candidate", "live", "reuse"} {
		t.Run(which, func(t *testing.T) {
			p := finalizedPair(t)
			if which != "reuse" {
				p.exp.PublicAddress = "127.0.0.2"
			}
			code, err := p.ex.PrepareEX(context.Background(), "changed", p.exp)
			if err != nil {
				t.Fatal(err)
			}
			reply, err := p.ir.PrepareIR(context.Background(), "changed", code, p.irp)
			if err != nil {
				t.Fatal(err)
			}
			dir := p.ex.pkiDir()
			if which == "candidate" {
				dir += ".next-changed"
			}
			if err := os.WriteFile(filepath.Join(dir, "server.pem"), []byte("operator edit"), 0644); err != nil {
				t.Fatal(err)
			}
			live, _ := snapshotPKI(p.ex.pkiDir())
			calls := len(p.ex.host.calls)
			config, _ := os.ReadFile(p.ex.liveConfig())
			if _, err := p.ex.CommitEX(context.Background(), "changed", reply); err == nil {
				t.Fatal("commit accepted changed prepared/live certificate material")
			}
			after, _ := snapshotPKI(p.ex.pkiDir())
			gotConfig, _ := os.ReadFile(p.ex.liveConfig())
			if !reflect.DeepEqual(live, after) || string(config) != string(gotConfig) || len(p.ex.host.calls) != calls {
				t.Fatal("refused commit changed live artifacts or service")
			}
			if p.ex.activeID() != "changed" {
				t.Fatal("refused commit lost transaction ownership")
			}
		})
	}
}
