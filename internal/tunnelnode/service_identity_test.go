package tunnelnode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func bindServiceIdentity(t *testing.T, n *node, identity string) {
	t.Helper()
	tx, err := n.readTxn("candidate")
	if err != nil {
		t.Fatal(err)
	}
	tx.PreparedServiceUser, tx.PreparedServiceIdentity, n.User = "root", identity, "root"
	if err := n.writeTxn(tx); err != nil {
		t.Fatal(err)
	}
}

func TestServiceIdentityIsSealedBeforeBothNodePreparations(t *testing.T) {
	if os.Geteuid() != 0 {
		if os.Getenv("BAFT_REQUIRE_SERVICE_ACCESS_EVIDENCE") == "1" {
			t.Fatal("ownership-bearing preparation fixture requires root")
		}
		t.Skip("ownership-bearing preparation fixture requires root; mandatory CI runs it as root")
	}
	p := finalizedPair(t)
	p.exp.StageConfig, p.irp.StageConfig = true, true
	identity := strings.Repeat("a", 64)
	for _, n := range []*node{p.ex, p.ir} {
		n.User = "root"
		n.System = &identityAccessSystem{System: n.System, identity: identity}
	}
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
	for i, n := range []*node{p.ex, p.ir} {
		tx, err := n.readTxn("candidate")
		if err != nil || tx.PreparedServiceUser != "root" || tx.PreparedServiceIdentity != identity {
			t.Fatal("prepared receipt did not seal service credentials", err)
		}
		gate := n.System.(*identityAccessSystem)
		if len(gate.args) < 5 || gate.args[0] != "-I" || gate.args[4] != identity {
			t.Fatal("helper was not isolated or bound to prepared credentials")
		}
		// Reopen from the persisted receipt rather than a manager-local cache.
		fresh, err := New(n.Env)
		if err != nil {
			t.Fatal(err)
		}
		r := []CandidateReport{ex, ir}[i]
		public, _ := json.Marshal(r)
		if strings.Contains(string(public), identity) || strings.Contains(string(public), "service_identity") {
			t.Fatal("private credential seal left the node in a candidate report")
		}
		if _, err := fresh.CommitCandidate(ctx, "candidate", r.ConfigSHA256, r.Generation); err != nil {
			t.Fatal("unchanged persisted credentials could not activate", err)
		}
	}
}

func TestServiceIdentityDriftAtEachActivationBoundary(t *testing.T) {
	for _, role := range []string{RoleEX, RoleIR} {
		for boundary := 1; boundary <= 5; boundary++ {
			t.Run(fmt.Sprintf("%s/check-%d", role, boundary), func(t *testing.T) {
				p, _, ex, ir := stageCandidatePair(t)
				n, report := p.ex, ex
				if role == RoleIR {
					n, report = p.ir, ir
				}
				identity := strings.Repeat("a", 64)
				bindServiceIdentity(t, n, identity)
				gate := &identityAccessSystem{System: n.System, identity: identity, changeAt: boundary}
				n.System = gate
				assertPreparedRefusal(t, n, func() error {
					_, err := n.CommitCandidate(context.Background(), "candidate", report.ConfigSHA256, report.Generation)
					if !errors.Is(err, errServiceIdentityChanged) {
						t.Errorf("identity drift did not explain refusal: %v", err)
					}
					return err
				})
				if gate.calls != boundary {
					t.Fatalf("credential drift reached later boundary: calls=%d", gate.calls)
				}
			})
		}
	}
}

func TestServiceIdentityUnknownRefusesBeforeNodeClaim(t *testing.T) {
	for _, output := range []string{"", "0", strings.Repeat("A", 64), strings.Repeat("a", 65), strings.Repeat("a", 64) + "\nBAFTREPLY1:private-fixture"} {
		n := newNode(t)
		n.User = "root"
		n.System = &identityAccessSystem{System: n.System, identity: output}
		_, err := n.PrepareEX(context.Background(), "candidate", ExParams{PublicAddress: "localhost", Port: 8443, Target: "127.0.0.1:2443", RouteID: "service-main", StageConfig: true})
		if err == nil || n.activeID() != "" || n.host.called("restart") || strings.Contains(err.Error(), "private-fixture") {
			t.Fatal("unknown identity claimed the node, changed service or leaked helper detail")
		}
	}
}

func TestServiceIdentityOlderAccountReceiptKeepsAccessContract(t *testing.T) {
	p, _, _, _ := stageCandidatePair(t)
	bindServiceIdentity(t, p.ir, "")
	gate := &identityAccessSystem{System: p.ir.System, identity: strings.Repeat("b", 64)}
	p.ir.System = gate
	if _, err := p.ir.ValidateCandidate(context.Background(), "candidate"); err != nil {
		t.Fatal("older account receipt silently acquired identity gate", err)
	}
	if gate.calls != 2 || len(gate.args) <= 5 || gate.args[4] != "" {
		t.Fatal("older receipt lost its account-access checks")
	}
}

func TestServiceIdentityInvalidReceiptAndCancellationRefuse(t *testing.T) {
	m := newNode(t).Manager
	gate := &identityAccessSystem{System: m.System, identity: strings.Repeat("a", 64)}
	m.System = gate
	if _, err := m.serviceAccountIdentity(context.Background(), "root", "invalid", nil); err == nil || gate.calls != 0 {
		t.Fatal("invalid stored identity was probed or accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.serviceAccountIdentity(ctx, "root", "", nil); err == nil {
		t.Fatal("cancelled identity check accepted adapter success")
	}
}

// A controlled subprocess models NSS changes without editing host accounts.
// Native acceptance separately exercises actual initgroups/setgid/setuid.
func TestServiceIdentityHelperDetectsUIDGIDAndGroupChanges(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Fatal("service-identity helper requires python3", err)
	}
	dir := t.TempDir()
	for _, name := range []string{"hashlib.py", "json.py", "pwd.py"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("raise RuntimeError('private-fixture-import')\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	expected := shaHex([]byte("[1,11,17,[17,23,29]]"))
	for _, c := range []struct {
		name     string
		uid, gid int
		groups   string
		pass     bool
	}{{"same", 11, 17, "[17,23,29]", true}, {"group-order-and-duplicates", 11, 17, "[29,17,23,23]", true}, {"uid", 12, 17, "[17,23,29]", false}, {"gid", 11, 18, "[17,23,29]", false}, {"added-group", 11, 17, "[17,23,29,31]", false}, {"removed-group", 11, 17, "[17,29]", false}} {
		t.Run(c.name, func(t *testing.T) {
			prefix := fmt.Sprintf(`import os, pwd, types
calls = []
pwd.getpwnam = lambda name: types.SimpleNamespace(pw_name=name, pw_gid=17, pw_uid=11)
os.initgroups = lambda name, gid: calls.append(("groups", name, gid))
os.setgid = lambda gid: calls.append(("gid", gid))
os.setuid = lambda uid: calls.append(("uid", uid))
def effective_uid():
    assert calls == [("groups", "fixture", 17), ("gid", 17), ("uid", 11)]
    return %d
os.getuid = effective_uid
os.getgid = lambda: %d
os.getgroups = lambda: %s
`, c.uid, c.gid, c.groups)
			cmd := exec.Command("python3", "-I", "-c", prefix+serviceAccessProgram, "fixture", expected)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "PYTHONPATH="+dir)
			out, err := cmd.CombinedOutput()
			if c.pass {
				if err != nil || strings.TrimSpace(string(out)) != expected {
					t.Fatal("stable effective credentials refused or interpreter imports polluted", err)
				}
			} else if err == nil || strings.TrimSpace(string(out)) != "identity_changed" {
				t.Fatal("effective credential change was not refused")
			}
		})
	}
}
