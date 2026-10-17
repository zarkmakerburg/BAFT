package tunnelengine

import (
	"strings"
	"testing"
)

func TestFailClosedCatalog(t *testing.T) {
	for _, engine := range []string{"", "baft", " baft "} {
		got, err := RequireDeployable(engine)
		if err != nil || got != "baft" {
			t.Fatalf("%q -> %q, %v", engine, got, err)
		}
	}
	for _, engine := range []string{"frp", "rathole", "backhaul", "wireguard", "ssh-socks", "wss", "quic", "FRP", "unknown", "baft;frp"} {
		got, err := RequireDeployable(engine)
		if err == nil || got != "" {
			t.Fatalf("%q activated without a real adapter: %q, %v", engine, got, err)
		}
		if !strings.Contains(err.Error(), "engine") {
			t.Fatalf("error not actionable: %v", err)
		}
	}
	if caps := Available(); len(caps) != 8 || !caps[0].Deployable || caps[0].ContractVersion != DriverContractVersion {
		t.Fatalf("unexpected engine catalog: %+v", caps)
	} else {
		for _, cap := range caps[1:] {
			if cap.Deployable || cap.ContractVersion != 0 {
				t.Fatalf("planned driver incorrectly enabled: %+v", cap)
			}
		}
	}
}
