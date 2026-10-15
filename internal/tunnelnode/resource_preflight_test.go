package tunnelnode

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func TestResourcePreflightUnknownInventoryRetainsUntouchedNode(t *testing.T) {
	for _, mode := range []string{"error", "malformed", "unbounded", "self-collision", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			n := newNode(t)
			n.TCPInventory = func(context.Context) ([]TCPBinding, error) {
				switch mode {
				case "error":
					return nil, errors.New("inventory unavailable")
				case "malformed":
					return []TCPBinding{{Address: "127.0.0.1:8443", Inode: 0}}, nil
				case "unbounded":
					return make([]TCPBinding, 8193), nil
				}
				return nil, nil
			}
			p := ExParams{PublicAddress: "127.0.0.1", Port: 8443, Target: "127.0.0.1:2443", RouteID: "r", VerifyResources: true}
			if mode == "self-collision" {
				p.MetricsListen = "127.0.0.1:8443"
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancelled" {
				cancel()
			}
			if _, err := n.PrepareEX(ctx, "refused", p); err == nil {
				t.Fatal("unsafe or missing evidence accepted")
			}
			if _, err := os.Lstat(n.ConfigDir); !os.IsNotExist(err) {
				t.Fatal("refused preflight created configuration or keys")
			}
			if n.activeID() != "" || len(n.host.calls) != 0 {
				t.Fatal("refused preflight claimed a change or touched services")
			}
		})
	}
}

func TestResourcePreflightProtectsUnmanagedAndCorruptNamespaces(t *testing.T) {
	for _, mode := range []string{"unmanaged-unit", "unmanaged-config", "symlink-config-parent", "corrupt-active", "missing-active-transaction", "unknown-admin-file"} {
		t.Run(mode, func(t *testing.T) {
			n := newNode(t)
			n.TCPInventory = func(context.Context) ([]TCPBinding, error) { return nil, nil }
			var keepPath string
			switch mode {
			case "unmanaged-unit":
				keepPath = n.unitPath()
			case "unmanaged-config":
				keepPath = n.liveConfig()
			case "corrupt-active", "missing-active-transaction":
				keepPath = n.activePath()
			case "unknown-admin-file":
				keepPath = filepath.Join(n.StateDir, "admin.sock")
			case "symlink-config-parent":
				target := t.TempDir()
				if err := os.Symlink(target, n.ConfigDir); err != nil {
					t.Fatal(err)
				}
			}
			if keepPath != "" {
				b := []byte("operator-owned content")
				if mode == "missing-active-transaction" {
					b = []byte("missing-change\n")
				}
				if err := writeFile(keepPath, b, 0600); err != nil {
					t.Fatal(err)
				}
			}
			p := ExParams{PublicAddress: "127.0.0.1", Port: 8443, Target: "127.0.0.1:2443", RouteID: "r", VerifyResources: true}
			if _, err := n.PrepareEX(context.Background(), "refused", p); err == nil {
				t.Fatal("unknown resource namespace was adopted")
			}
			if keepPath != "" {
				b, err := os.ReadFile(keepPath)
				if err != nil || (!strings.Contains(string(b), "operator-owned") && !strings.Contains(string(b), "missing-change")) {
					t.Fatal("unknown resource was modified")
				}
			}
			if _, err := os.Lstat(n.stage("refused")); !os.IsNotExist(err) {
				t.Fatal("refused namespace acquired a stage")
			}
		})
	}
}

func TestResourcePreflightRechecksPortBeforeCommitAndAllowsRetry(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("passive inventory requires Linux")
	}
	p := newPair(t)
	p.exp.VerifyResources = true
	code, err := p.ex.PrepareEX(context.Background(), "late-bind", p.exp)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := p.ir.PrepareIR(context.Background(), "late-bind", code, p.irp)
	if err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(p.exp.Port)))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if _, err := p.ex.CommitEX(context.Background(), "late-bind", reply); err == nil || !strings.Contains(err.Error(), "occupied") {
		t.Fatalf("newly occupied bind accepted: %v", err)
	}
	for _, path := range []string{p.ex.liveConfig(), p.ex.unitPath(), p.ex.pkiDir(), p.ex.generationPath()} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatal("refused commit changed live state")
		}
	}
	if _, err := os.Stat(filepath.Join(p.ex.stage("late-bind"), "pending.json")); err != nil {
		t.Fatal("refused preflight consumed the pairing state")
	}
	if p.ex.activeID() != "late-bind" {
		t.Fatal("refused commit lost its pending claim")
	}
	l.Close()
	if _, err := p.ex.CommitEX(context.Background(), "late-bind", reply); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ex.Rollback(context.Background(), "late-bind"); err != nil {
		t.Fatal(err)
	}
}

type resourcePIDSystem struct {
	System
	pid string
}

func (s resourcePIDSystem) Systemctl(ctx context.Context, args ...string) (string, error) {
	if len(args) > 2 && args[0] == "show" && args[2] == "MainPID" {
		return s.pid, nil
	}
	return s.System.Systemctl(ctx, args...)
}

func TestResourcePreflightReuseNeedsArtifactAndSocketOwnerProof(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("passive inventory requires Linux")
	}
	for _, mode := range []string{"owned", "unknown-pid", "changed-unit", "changed-marker"} {
		t.Run(mode, func(t *testing.T) {
			p := finalizedPair(t)
			l, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(p.exp.Port)))
			if err != nil {
				t.Fatal(err)
			}
			defer l.Close()
			p.ex.System = resourcePIDSystem{System: p.ex.System, pid: strconv.Itoa(os.Getpid())}
			if mode == "unknown-pid" {
				p.ex.System = resourcePIDSystem{System: p.ex.System, pid: "0"}
			}
			if mode == "changed-unit" {
				if err := os.WriteFile(p.ex.unitPath(), []byte("operator edit"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "changed-marker" {
				if err := os.WriteFile(p.ex.markerPath(), []byte("{}"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			p.exp.VerifyResources = true
			_, err = p.ex.PrepareEX(context.Background(), "reuse", p.exp)
			if mode == "owned" {
				if err != nil {
					t.Fatal(err)
				}
				if _, err := p.ex.Rollback(context.Background(), "reuse"); err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("reuse accepted without current artifact/PID ownership proof")
			}
		})
	}
}

func TestResourcePreflightDoesNotUnlinkAnUnrelatedUnixSocket(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Unix socket fixture requires Linux")
	}
	n := newNode(t)
	if err := os.MkdirAll(n.StateDir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(n.StateDir, "admin.sock")
	l, err := net.Listen("unix", path)
	if err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			if os.Getenv("BAFT_REQUIRE_RESOURCE_EVIDENCE") == "1" {
				t.Fatalf("required Unix socket acceptance environment is unavailable: %v", err)
			}
			t.Skipf("environment cannot create Unix sockets: %v", err)
		}
		t.Fatal(err)
	}
	defer l.Close()
	n.TCPInventory = func(context.Context) ([]TCPBinding, error) { return nil, nil }
	p := ExParams{PublicAddress: "127.0.0.1", Port: 8443, Target: "127.0.0.1:2443", RouteID: "r", VerifyResources: true}
	if _, err := n.PrepareEX(context.Background(), "refused", p); err == nil {
		t.Fatal("unknown admin socket was adopted")
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatal("preflight unlinked another service's Unix socket")
	}
}
