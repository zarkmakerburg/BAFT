package tunnelnode

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// The JSON insertion lets the identical regression run on the old receipt
// schema too: it exposes silently ignored identity evidence on that baseline.
type identityAccessSystem struct {
	System
	identity string
	calls    int
	changeAt int
	args     []string
}

func (s *identityAccessSystem) Run(ctx context.Context, bin string, args ...string) (string, error) {
	if bin == "python3" {
		s.calls++
		s.args = append([]string(nil), args...)
		if s.changeAt > 0 && s.calls >= s.changeAt {
			return strings.Repeat("b", 64), nil
		}
		return s.identity, nil
	}
	return s.System.Run(ctx, bin, args...)
}

func TestCandidateRefusesSameNameAccountIdentityDrift(t *testing.T) {
	for _, role := range []string{RoleEX, RoleIR} {
		t.Run(role, func(t *testing.T) {
			p, _, ex, ir := stageCandidatePair(t)
			n, report := p.ex, ex
			if role == RoleIR {
				n, report = p.ir, ir
			}
			tx, err := n.readTxn("candidate")
			if err != nil {
				t.Fatal(err)
			}
			tx.PreparedServiceUser, n.User = "root", "root"
			encoded, err := json.Marshal(tx)
			if err != nil {
				t.Fatal(err)
			}
			var receipt map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &receipt); err != nil {
				t.Fatal(err)
			}
			receipt["prepared_service_identity"], _ = json.Marshal(strings.Repeat("a", 64))
			encoded, _ = json.Marshal(receipt)
			if err := json.Unmarshal(encoded, &tx); err != nil {
				t.Fatal(err)
			}
			if err := n.writeTxn(tx); err != nil {
				t.Fatal(err)
			}
			n.System = &identityAccessSystem{System: n.System, identity: strings.Repeat("b", 64)}
			before, calls := liveArtifacts(t, n), len(n.host.calls)
			_, err = n.CommitCandidate(context.Background(), "candidate", report.ConfigSHA256, report.Generation)
			if err == nil {
				t.Errorf("same-name account identity drift accepted: artifacts_changed=%t service_calls=%v", !reflect.DeepEqual(before, liveArtifacts(t, n)), n.host.calls[calls:])
			}
			if !reflect.DeepEqual(before, liveArtifacts(t, n)) || n.activeID() != "candidate" {
				t.Error("account identity drift changed live artifacts or consumed preparation")
			}
		})
	}
}
