package tunnelnode

import (
	"context"
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type deniedServiceAccess struct {
	System
	output string
	calls  int
	args   []string
}

func (s *deniedServiceAccess) Run(ctx context.Context, bin string, args ...string) (string, error) {
	if bin == "python3" {
		s.calls++
		s.args = append([]string(nil), args...)
		return s.output, errors.New("access denied")
	}
	return s.System.Run(ctx, bin, args...)
}

func TestCandidateServiceAccessRefusalKeepsBothActiveNodes(t *testing.T) {
	p, _, ex, ir := stageCandidatePair(t)
	for _, side := range []struct {
		n      *node
		report CandidateReport
		count  int
	}{{p.ex, ex, 7}, {p.ir, ir, 5}} {
		tx, err := side.n.readTxn("candidate")
		if err != nil {
			t.Fatal(err)
		}
		tx.PreparedServiceUser = "root"
		side.n.User = "root"
		if err := side.n.writeTxn(tx); err != nil {
			t.Fatal(err)
		}
		gate := &deniedServiceAccess{System: side.n.System}
		side.n.System = gate
		for i := 0; i < side.count; i++ {
			gate.output = strconv.Itoa(i)
			assertPreparedRefusal(t, side.n, func() error { _, err := side.n.ValidateCandidate(context.Background(), "candidate"); return err })
			assertPreparedRefusal(t, side.n, func() error {
				_, err := side.n.CommitCandidate(context.Background(), "candidate", side.report.ConfigSHA256, side.report.Generation)
				return err
			})
		}
		if gate.calls != 2*side.count || len(gate.args) != 3+2*side.count {
			t.Fatal("not every account-access check was exercised")
		}
		if side.n == p.ex && !strings.Contains(strings.Join(gate.args, " "), p.ex.preparedPKIDir(tx)) {
			t.Fatal("EX did not probe prepared PKI")
		}
		gate.output = "BAFTREPLY1:secret-must-never-leave-helper"
		_, err = side.n.ValidateCandidate(context.Background(), "candidate")
		if err == nil || strings.Contains(err.Error(), "secret-must") {
			t.Fatal("helper detail leaked or unsupported account was accepted")
		}
		gate.calls = 0
		side.n.User = "changed-account"
		assertPreparedRefusal(t, side.n, func() error {
			_, err := side.n.CommitCandidate(context.Background(), "candidate", side.report.ConfigSHA256, side.report.Generation)
			return err
		})
		if gate.calls != 0 {
			t.Fatal("changed service account was probed instead of refused")
		}
	}
}

func TestServiceAccessUnavailableDoesNotClaimNode(t *testing.T) {
	n := newNode(t)
	n.User = "unavailable-account"
	n.System = &deniedServiceAccess{System: n.System, output: ""}
	_, err := n.PrepareEX(context.Background(), "candidate", ExParams{PublicAddress: "localhost", Port: 8443, Target: "127.0.0.1:2443", RouteID: "service-main", StageConfig: true})
	if err == nil || n.activeID() != "" || n.host.called("restart") {
		t.Fatal("unavailable service account claimed or changed node")
	}
}

func TestOlderCandidateDoesNotAcquireServiceAccessGate(t *testing.T) {
	p, _, _, _ := stageCandidatePair(t)
	p.ir.User = "changed-account"
	p.ir.System = &deniedServiceAccess{System: p.ir.System, output: ""}
	if _, err := p.ir.ValidateCandidate(context.Background(), "candidate"); err != nil {
		t.Fatal("older receipt silently upgraded", err)
	}
}

// Required root-only acceptance runs explicitly in CI; local inability to
// switch uid/groups must not be presented as a successful service-user probe.
func TestServiceAccessRealCredentialSwitch(t *testing.T) {
	require := os.Getenv("BAFT_REQUIRE_SERVICE_ACCESS_EVIDENCE") == "1"
	unavailable := func(msg string) {
		if require {
			t.Fatal(msg)
		}
		t.Skip(msg)
	}
	if os.Geteuid() != 0 {
		unavailable("real service-user credential switch requires root")
	}
	if _, err := user.Lookup("nobody"); err != nil {
		unavailable("test service account nobody is unavailable")
	}
	dir, err := os.MkdirTemp("", "baft-account-access-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "private-key")
	if err := os.WriteFile(path, []byte("synthetic fixture bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	m := newNode(t).Manager
	if err := m.checkServiceAccess(context.Background(), "nobody", nil); err != nil {
		unavailable("environment cannot switch service account/groups: " + err.Error())
	}
	check := []serviceAccessCheck{{"r", path, "private key"}}
	if err := m.checkServiceAccess(context.Background(), "nobody", check); err == nil {
		t.Fatal("service account read root-only fixture")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if err := m.checkServiceAccess(context.Background(), "nobody", check); err != nil {
		t.Fatal("service account could not read accessible fixture", err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := m.checkServiceAccess(context.Background(), "nobody", check); err == nil {
		t.Fatal("unsearchable parent directory accepted")
	}
}
