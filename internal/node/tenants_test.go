package node

import (
	"testing"

	"github.com/zarkmakerburg/baft/internal/config"
)

func TestTenantFlowLimitDefaultsOnlyForSharedListeners(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Config
		want int
	}{
		{
			name: "legacy single peer remains uncapped",
			cfg: config.Config{
				Server: &config.Server{AllowedPeerIdentities: []string{"urn:baft:node:ir-one"}},
				Limits: config.Limits{MaxFlows: 256},
			},
			want: 0,
		},
		{
			name: "shared listener reserves a quarter of flow budget per tenant",
			cfg: config.Config{
				Server: &config.Server{AllowedPeerIdentities: []string{"urn:baft:node:ir-one", "urn:baft:node:ir-two"}},
				Limits: config.Limits{MaxFlows: 256},
			},
			want: 64,
		},
		{
			name: "small shared listener still gets a positive tenant cap",
			cfg: config.Config{
				Server: &config.Server{AllowedPeerIdentities: []string{"urn:baft:node:ir-one", "urn:baft:node:ir-two"}},
				Limits: config.Limits{MaxFlows: 3},
			},
			want: 1,
		},
		{
			name: "explicit tenant cap takes precedence",
			cfg: config.Config{
				Server: &config.Server{AllowedPeerIdentities: []string{"urn:baft:node:ir-one", "urn:baft:node:ir-two"}},
				Limits: config.Limits{MaxFlows: 256, MaxFlowsPerPeer: 19},
			},
			want: 19,
		},
		{
			name: "nonpositive node capacity leaves cap disabled",
			cfg: config.Config{
				Server: &config.Server{AllowedPeerIdentities: []string{"urn:baft:node:ir-one", "urn:baft:node:ir-two"}},
				Limits: config.Limits{MaxFlows: 0},
			},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tenantFlowLimit(tt.cfg); got != tt.want {
				t.Fatalf("tenantFlowLimit() = %d, want %d", got, tt.want)
			}
		})
	}
}
