package tunnelnode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecutableIdentityRefusesUnsafeFiles(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "binary")
	if err := os.WriteFile(path, []byte("real executable bytes"), 0700); err != nil {
		t.Fatal(err)
	}
	want, err := executableIdentity(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"missing", "symlink", "directory", "fifo", "empty", "non-executable", "oversized", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			candidate := filepath.Join(t.TempDir(), "candidate")
			testCtx := ctx
			switch mode {
			case "symlink":
				if err := os.Symlink(path, candidate); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(candidate, 0700); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := syscallMkfifo(candidate); err != nil {
					t.Fatal(err)
				}
			case "empty":
				if err := os.WriteFile(candidate, nil, 0700); err != nil {
					t.Fatal(err)
				}
			case "non-executable":
				if err := os.WriteFile(candidate, []byte("bytes"), 0600); err != nil {
					t.Fatal(err)
				}
			case "oversized":
				f, err := os.OpenFile(candidate, os.O_CREATE|os.O_WRONLY, 0700)
				if err != nil {
					t.Fatal(err)
				}
				if err = f.Truncate((128 << 20) + 1); err != nil {
					t.Fatal(err)
				}
				f.Close()
			case "cancelled":
				var cancel context.CancelFunc
				testCtx, cancel = context.WithCancel(ctx)
				cancel()
				candidate = path
			}
			if _, err := executableIdentity(testCtx, candidate); err == nil {
				t.Fatal("unsafe executable accepted")
			}
		})
	}
	got, err := executableIdentity(ctx, path)
	if err != nil || got != want {
		t.Fatal("identity check modified executable")
	}
}

func privateExecutables(t *testing.T, n *node) {
	t.Helper()
	for _, p := range []*string{&n.BaftBin, &n.PairBin} {
		b, err := os.ReadFile(*p)
		if err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(t.TempDir(), filepath.Base(*p))
		if err := os.WriteFile(dest, b, 0700); err != nil {
			t.Fatal(err)
		}
		*p = dest
	}
}

func TestCandidateExecutableDriftPreservesActiveService(t *testing.T) {
	for _, role := range []string{RoleEX, RoleIR} {
		for _, binary := range []string{"baft", "pair"} {
			for _, mode := range []string{"replacement", "mode", "symlink", "path"} {
				t.Run(role+"/"+binary+"/"+mode, func(t *testing.T) {
					p := finalizedPair(t)
					p.exp.StageConfig, p.irp.StageConfig = true, true
					p.exp.PublicAddress = "localhost"
					n := p.ex
					if role == RoleIR {
						n = p.ir
					}
					privateExecutables(t, n)
					ctx := context.Background()
					code, err := p.ex.PrepareEX(ctx, "candidate", p.exp)
					if err != nil {
						t.Fatal(err)
					}
					reply, err := p.ir.PrepareIR(ctx, "candidate", code, p.irp)
					if err != nil {
						t.Fatal(err)
					}
					ex, err := p.ex.StageEX(ctx, "candidate", reply)
					if err != nil {
						t.Fatal(err)
					}
					ir, err := p.ir.ValidateCandidate(ctx, "candidate")
					if err != nil {
						t.Fatal(err)
					}
					report := ex
					if role == RoleIR {
						report = ir
					}
					path := &n.BaftBin
					if binary == "pair" {
						path = &n.PairBin
					}
					switch mode {
					case "replacement":
						if err := os.WriteFile(*path, []byte("replacement installer binary"), 0700); err != nil {
							t.Fatal(err)
						}
					case "mode":
						if err := os.Chmod(*path, 0755); err != nil {
							t.Fatal(err)
						}
					case "symlink":
						orig := *path + ".orig"
						if err := os.Rename(*path, orig); err != nil {
							t.Fatal(err)
						}
						if err := os.Symlink(orig, *path); err != nil {
							t.Fatal(err)
						}
					case "path":
						b, err := os.ReadFile(*path)
						if err != nil {
							t.Fatal(err)
						}
						dest := *path + ".other"
						if err := os.WriteFile(dest, b, 0700); err != nil {
							t.Fatal(err)
						}
						*path = dest
					}
					assertPreparedRefusal(t, n, func() error { _, err := n.ValidateCandidate(ctx, "candidate"); return err })
					assertPreparedRefusal(t, n, func() error {
						_, err := n.CommitCandidate(ctx, "candidate", report.ConfigSHA256, report.Generation)
						return err
					})
				})
			}
		}
	}
}

type executableDriftValidator struct {
	System
	path    string
	changed bool
}

func (s *executableDriftValidator) Run(ctx context.Context, bin string, args ...string) (string, error) {
	out, err := s.System.Run(ctx, bin, args...)
	if err == nil && len(args) > 1 && args[0] == "config" && args[1] == "validate" {
		err = os.Chmod(s.path, 0755)
		s.changed = err == nil
	}
	return out, err
}
func TestCandidateValidatorExecutableDriftRefusedBeforeActivation(t *testing.T) {
	p := finalizedPair(t)
	privateExecutables(t, p.ex)
	p.exp.StageConfig, p.irp.StageConfig = true, true
	p.exp.PublicAddress = "localhost"
	ctx := context.Background()
	code, err := p.ex.PrepareEX(ctx, "candidate", p.exp)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := p.ir.PrepareIR(ctx, "candidate", code, p.irp)
	if err != nil {
		t.Fatal(err)
	}
	report, err := p.ex.StageEX(ctx, "candidate", reply)
	if err != nil {
		t.Fatal(err)
	}
	validator := &executableDriftValidator{System: p.ex.System, path: p.ex.BaftBin}
	p.ex.System = validator
	assertPreparedRefusal(t, p.ex, func() error {
		_, err := p.ex.CommitCandidate(ctx, "candidate", report.ConfigSHA256, report.Generation)
		return err
	})
	if !validator.changed {
		t.Fatal("validator did not inject drift")
	}
}
func TestCandidatePreparationMissingExecutableDoesNotClaimNode(t *testing.T) {
	n := newNode(t)
	n.BaftBin = filepath.Join(t.TempDir(), "missing")
	_, err := n.PrepareEX(context.Background(), "candidate", ExParams{PublicAddress: "localhost", Port: 8443, Target: "127.0.0.1:2443", RouteID: "service-main", StageConfig: true})
	if err == nil || !strings.Contains(err.Error(), "executable") {
		t.Fatalf("got %v", err)
	}
	if n.activeID() != "" || n.host.called("restart") {
		t.Fatal("missing executable claimed node or changed service")
	}
}

func TestCandidatePairExecutableDriftBeforeReplyConsumption(t *testing.T) {
	p := finalizedPair(t)
	privateExecutables(t, p.ex)
	p.exp.StageConfig, p.irp.StageConfig = true, true
	p.exp.PublicAddress = "localhost"
	ctx := context.Background()
	code, err := p.ex.PrepareEX(ctx, "candidate", p.exp)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := p.ir.PrepareIR(ctx, "candidate", code, p.irp)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.ex.PairBin, []byte("replacement"), 0700); err != nil {
		t.Fatal(err)
	}
	assertPreparedRefusal(t, p.ex, func() error { _, err := p.ex.StageEX(ctx, "candidate", reply); return err })
	for _, name := range []string{"pending.json", "psk"} {
		if _, err := os.Stat(filepath.Join(p.ex.stage("candidate"), name)); err != nil {
			t.Fatal("refusal consumed pairing material", err)
		}
	}
}

func TestExecutableIdentityAllowsIdenticalAtomicReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "binary")
	next := path + ".next"
	for _, p := range []string{path, next} {
		if err := os.WriteFile(p, []byte("identical bytes"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	before, err := executableIdentity(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(next, path); err != nil {
		t.Fatal(err)
	}
	after, err := executableIdentity(context.Background(), path)
	if err != nil || before != after {
		t.Fatal("identical binary replacement changed identity", err)
	}
}

func TestOlderCandidateReceiptDoesNotAcquireExecutableGate(t *testing.T) {
	p, _, _, _ := stageCandidatePair(t)
	tx, err := p.ir.readTxn("candidate")
	if err != nil {
		t.Fatal(err)
	}
	tx.PreparedExecutables = nil
	if err := p.ir.writeTxn(tx); err != nil {
		t.Fatal(err)
	}
	p.ir.PairBin = filepath.Join(t.TempDir(), "absent-older-pair")
	if _, err := p.ir.ValidateCandidate(context.Background(), "candidate"); err != nil {
		t.Fatal("silently upgraded older receipt", err)
	}
}
