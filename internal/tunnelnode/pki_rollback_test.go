package tunnelnode

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

func changedHostPair(t *testing.T) (*pair, map[string]string) {
	t.Helper()
	p := newPair(t)
	p.build(t, "old-host")
	for _, n := range []*node{p.ex, p.ir} {
		if _, err := n.Finalize(context.Background(), "old-host"); err != nil {
			t.Fatal(err)
		}
	}
	old := pkiDigests(p.ex.pkiDir())
	p.exp.PublicAddress = "127.0.0.2"
	p.build(t, "new-host")
	return p, old
}

func TestPKIRollbackRestoresPreviousCertificateBeforeRestart(t *testing.T) {
	p, old := changedHostPair(t)
	restarted := false
	p.ex.host.onRestart = func() error {
		restarted = true
		cert, err := tls.LoadX509KeyPair(filepath.Join(p.ex.pkiDir(), "server.pem"), filepath.Join(p.ex.pkiDir(), "server.key"))
		if err != nil {
			return err
		}
		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return err
		}
		if err := leaf.VerifyHostname("127.0.0.1"); err != nil || !sameDigests(pkiDigests(p.ex.pkiDir()), old) {
			return errors.New("old service started with new certificate material")
		}
		return nil
	}
	if _, err := p.ex.Rollback(context.Background(), "new-host"); err != nil {
		t.Fatal(err)
	}
	if !restarted || !p.ex.host.active {
		t.Fatal("rollback did not start the previous service with its previous certificate")
	}
}

func TestPKIRollbackRefusesUnverifiedMaterialBeforeChangingLiveState(t *testing.T) {
	for _, scenario := range []string{"missing-backup", "changed-backup", "changed-live", "extra-live-file", "symlink-backup", "symlink-live-file", "changed-live-mode", "changed-live-owner", "missing-identity"} {
		t.Run(scenario, func(t *testing.T) {
			if scenario == "changed-live-owner" && os.Geteuid() != 0 {
				t.Skip("changing ownership requires root")
			}
			p, _ := changedHostPair(t)
			before := p.ex.pkiDir() + ".before-new-host"
			switch scenario {
			case "missing-backup":
				if err := os.RemoveAll(before); err != nil {
					t.Fatal(err)
				}
			case "changed-backup":
				if err := os.WriteFile(filepath.Join(before, "ca.pem"), []byte("operator backup edit"), 0644); err != nil {
					t.Fatal(err)
				}
			case "changed-live":
				if err := os.WriteFile(filepath.Join(p.ex.pkiDir(), "server.pem"), []byte("operator live edit"), 0644); err != nil {
					t.Fatal(err)
				}
			case "extra-live-file":
				if err := os.WriteFile(filepath.Join(p.ex.pkiDir(), "manual.txt"), []byte("keep this file"), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink-backup":
				if err := os.Rename(before, before+".operator"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(before+".operator", before); err != nil {
					t.Fatal(err)
				}
			case "symlink-live-file":
				path := filepath.Join(p.ex.pkiDir(), "ca.pem")
				if err := os.Rename(path, path+".operator"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+".operator", path); err != nil {
					t.Fatal(err)
				}
			case "changed-live-mode":
				if err := os.Chmod(filepath.Join(p.ex.pkiDir(), "server.key"), 0644); err != nil {
					t.Fatal(err)
				}
			case "changed-live-owner":
				if err := os.Chown(filepath.Join(p.ex.pkiDir(), "server.key"), 65534, 65534); err != nil {
					if errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.EPERM) {
						t.Skipf("environment cannot change uid/gid: %v", err)
					}
					t.Fatal(err)
				}
			case "missing-identity":
				tx, err := p.ex.readTxn("new-host")
				if err != nil {
					t.Fatal(err)
				}
				tx.PreviousPKI = nil // an older in-flight transaction
				if err := p.ex.writeTxn(tx); err != nil {
					t.Fatal(err)
				}
			}
			files := map[string]string{}
			for _, path := range []string{p.ex.liveConfig(), p.ex.unitPath(), p.ex.markerPath(), p.ex.generationPath()} {
				b, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				files[path] = string(b)
			}
			live := pkiDigests(p.ex.pkiDir())
			calls := len(p.ex.host.calls)
			if msg, err := p.ex.Rollback(context.Background(), "new-host"); err == nil {
				t.Fatalf("unsafe rollback acknowledged: %q", msg)
			}
			for path, want := range files {
				if b, err := os.ReadFile(path); err != nil || string(b) != want {
					t.Fatalf("refused rollback changed %s", path)
				}
			}
			if !reflect.DeepEqual(live, pkiDigests(p.ex.pkiDir())) || len(p.ex.host.calls) != calls || p.ex.activeID() != "new-host" {
				t.Fatal("refused rollback changed PKI/service or released ownership")
			}
		})
	}
}

func TestPKIRollbackRetriesAfterInterruptedExchangeWithoutBackup(t *testing.T) {
	p, old := changedHostPair(t)
	p.ex.Fault = func(point string) error {
		if point == "tunnel-rollback:after-pki-restore" {
			return errors.New("injected interruption after certificate exchange")
		}
		return nil
	}
	if _, err := p.ex.Rollback(context.Background(), "new-host"); err == nil {
		t.Fatal("interrupted rollback acknowledged")
	}
	if !sameDigests(pkiDigests(p.ex.pkiDir()), old) || p.ex.activeID() != "new-host" {
		t.Fatal("interrupted rollback lost the old tree or its claim")
	}
	// The verified old tree is now live. Losing the exchanged-out new tree
	// does not invalidate that durable proof of restoration.
	if err := os.RemoveAll(p.ex.pkiDir() + ".before-new-host"); err != nil {
		t.Fatal(err)
	}
	m, err := New(p.ex.Env)
	if err != nil {
		t.Fatal(err)
	}
	m.Fault = nil
	if msg, err := m.Rollback(context.Background(), "new-host"); err != nil || msg != "rolled back" {
		t.Fatalf("retry did not recognize the restored old tree: %q %v", msg, err)
	}
}

type pkiPrepareFailureSystem struct {
	System
	dir  string
	mode string
}

func (s *pkiPrepareFailureSystem) Run(ctx context.Context, name string, args ...string) (string, error) {
	if len(args) == 0 || args[0] != "pki" {
		return s.System.Run(ctx, name, args...)
	}
	dir := s.dir
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--dir" {
			dir = args[i+1]
		}
	}
	switch s.mode {
	case "partial":
		if err := os.Mkdir(dir, 0750); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dir, "server.key"), []byte("partial generated fixture"), 0600); err != nil {
			return "", err
		}
	case "generated", "changed-live":
		if out, err := s.System.Run(ctx, name, args...); err != nil {
			return out, err
		}
		if s.mode == "changed-live" {
			if err := os.WriteFile(filepath.Join(s.dir, "host"), []byte("operator edit"), 0644); err != nil {
				return "", err
			}
		}
	}
	return "", errors.New("injected PKI generation failure")
}

func TestPKIPrepareFailureRestoresOldMaterialOrReportsIncompleteAbort(t *testing.T) {
	for _, mode := range []string{"absent", "partial", "generated", "changed-live"} {
		t.Run(mode, func(t *testing.T) {
			p := newPair(t)
			p.build(t, "old-host")
			for _, n := range []*node{p.ex, p.ir} {
				if _, err := n.Finalize(context.Background(), "old-host"); err != nil {
					t.Fatal(err)
				}
			}
			old := pkiDigests(p.ex.pkiDir())
			p.ex.System = &pkiPrepareFailureSystem{System: p.ex.System, dir: p.ex.pkiDir(), mode: mode}
			p.exp.PublicAddress = "127.0.0.2"
			_, err := p.ex.PrepareEX(context.Background(), "prepare-fail", p.exp)
			if err == nil || !strings.Contains(err.Error(), "injected PKI generation failure") {
				t.Fatalf("prepare failure hidden: %v", err)
			}
			if mode == "changed-live" {
				if !strings.Contains(err.Error(), "rollback incomplete") || p.ex.activeID() != "prepare-fail" {
					t.Fatal("failed abort hidden or its ownership released")
				}
			} else if !sameDigests(pkiDigests(p.ex.pkiDir()), old) || p.ex.activeID() != "" {
				t.Fatal("failed preparation did not restore the old material and release the change")
			}
		})
	}
}

func TestPKIPrepareRefusesOccupiedBackupPath(t *testing.T) {
	p := newPair(t)
	p.build(t, "old-host")
	for _, n := range []*node{p.ex, p.ir} {
		if _, err := n.Finalize(context.Background(), "old-host"); err != nil {
			t.Fatal(err)
		}
	}
	old := pkiDigests(p.ex.pkiDir())
	before := p.ex.pkiDir() + ".before-occupied"
	if err := os.Mkdir(before, 0701); err != nil {
		t.Fatal(err)
	}
	p.exp.PublicAddress = "127.0.0.2"
	if _, err := p.ex.PrepareEX(context.Background(), "occupied", p.exp); err == nil {
		t.Fatal("preparation replaced an existing backup path")
	}
	info, err := os.Stat(before)
	if err != nil || info.Mode().Perm() != 0701 || !sameDigests(pkiDigests(p.ex.pkiDir()), old) {
		t.Fatal("refused preparation changed existing PKI or backup directory")
	}
}

func TestPKIRollbackDoesNotCompleteIfCertificatesChangeDuringRestart(t *testing.T) {
	p, _ := changedHostPair(t)
	path := filepath.Join(p.ex.pkiDir(), "server.pem")
	p.ex.host.onRestart = func() error {
		return os.WriteFile(path, []byte("operator edit during restart"), 0644)
	}
	if _, err := p.ex.Rollback(context.Background(), "new-host"); err == nil {
		t.Fatal("rollback acknowledged certificate changes during restart")
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != "operator edit during restart" {
		t.Fatal("rollback overwrote the operator certificate edit")
	}
	if p.ex.activeID() != "new-host" {
		t.Fatal("incomplete rollback released its claim")
	}
	if _, err := os.Stat(p.ex.pkiDir() + ".before-new-host"); err != nil {
		t.Fatal("incomplete rollback discarded the exchanged-out tree")
	}
}

func TestPKIRollbackRetriesAfterRestorationAndRestartFailure(t *testing.T) {
	p, old := changedHostPair(t)
	base := p.ex.System
	p.ex.System = &rollbackFailureSystem{System: base, verb: "restart"}
	if msg, err := p.ex.Rollback(context.Background(), "new-host"); err == nil || !strings.Contains(msg, "incomplete") {
		t.Fatalf("restart failure acknowledged: %q %v", msg, err)
	}
	if !sameDigests(pkiDigests(p.ex.pkiDir()), old) || p.ex.activeID() != "new-host" {
		t.Fatal("incomplete rollback lost restored certificates or active receipt")
	}
	// A fresh Manager reads the persisted identity, rather than relying on
	// in-memory progress to decide that the missing old backup is safe.
	m, err := New(p.ex.Env)
	if err != nil {
		t.Fatal(err)
	}
	m.System = base
	if msg, err := m.Rollback(context.Background(), "new-host"); err != nil || msg != "rolled back" {
		t.Fatalf("retry after certificate restoration failed: %q %v", msg, err)
	}
	if !sameDigests(pkiDigests(m.pkiDir()), old) || !p.ex.host.active {
		t.Fatal("retry did not restore the previous working service")
	}
}
