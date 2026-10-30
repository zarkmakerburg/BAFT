package agent

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

type capRoundTripper func(*http.Request) (*http.Response, error)

func (f capRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPollAdvertisesOperationalCapabilityOnlyWhenProvisioned(t *testing.T) {
	cases := []struct {
		name      string
		configure bool
		policyErr error
		want      []string
	}{
		{"provisioned", true, nil, []string{CapabilityOperationalV1}},
		{"no trusted policy", true, errors.New("no policy"), nil},
		{"no operational config", false, nil, nil},
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
			_, _ = h.agent.pull(context.Background())
			if !seen || len(got) != len(tc.want) || (len(got) == 1 && got[0] != tc.want[0]) {
				t.Fatalf("header values %v want %v (seen=%v)", got, tc.want, seen)
			}
		})
	}
}
