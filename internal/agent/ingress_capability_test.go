package agent

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressinventory"
	"github.com/zarkmakerburg/baft/internal/pathprobetls"
)

type capRoundTripper func(*http.Request) (*http.Response, error)

// agentNativeCaps: the features every agent of this build advertises,
// whatever binary or policy is installed (speed and TLS probe flows).
var agentNativeCaps = CapabilityPathProbeSpeedV1 + "," + pathprobetls.Capability

func (f capRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPollAdvertisesOperationalCapabilityOnlyWhenProvisioned(t *testing.T) {
	cases := []struct {
		name      string
		configure bool
		policyErr error
		want      []string
		profile   string
	}{
		// The agent-native speed-probe feature is always advertised; the
		// operational ingress capability only with a provisioned path.
		{"provisioned", true, nil, []string{CapabilityOperationalV1 + "," + agentNativeCaps}, ""},
		// ADR-003 safe host mode is a different contract with its own token;
		// it never also claims the LAB token.
		{"provisioned host mode", true, nil, []string{CapabilityOperationalHostV1 + "," + agentNativeCaps}, ingressinventory.ExecutionProfileHostV1},
		{"host mode without trusted policy", true, errors.New("no policy"), []string{agentNativeCaps}, ingressinventory.ExecutionProfileHostV1},
		{"no trusted policy", true, errors.New("no policy"), []string{agentNativeCaps}, ""},
		{"no operational config", false, nil, []string{agentNativeCaps}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newOperationalHarness(t, "public_ingress_apply", tc.configure)
			var got []string
			var seen bool
			base := h.agent.cfg.HTTP.Transport
			h.agent.cfg.HTTP.Transport = capRoundTripper(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/api/agent/jobs" {
					seen = true
					got = r.Header.Values(CapabilityHeader)
				}
				return base.RoundTrip(r)
			})
			h.agent.policyProvisioned = func(context.Context, string) error { return tc.policyErr }
			if h.agent.cfg.IngressOperational != nil {
				h.agent.cfg.IngressOperational.Profile = tc.profile
			}
			_, _ = h.agent.pull(context.Background())
			if !seen || len(got) != len(tc.want) || (len(got) == 1 && got[0] != tc.want[0]) {
				t.Fatalf("header values %v want %v (seen=%v)", got, tc.want, seen)
			}
		})
	}
}
