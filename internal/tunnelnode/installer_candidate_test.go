package tunnelnode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// #197 uses standalone baft-NAME directories with the same unit suffix as
// BCC's nested instances. That external unit must not be adopted even when
// it advertises BAFT ownership. This models its namespace contract, not a
// full installer run or mixed/shared-binary compatibility acceptance.
func TestResourceInstallerStandaloneUnitIsNotAdoptedByCandidate(t *testing.T) {
	n := newNode(t)
	m, err := n.ForInstance("bulgaria", "127.0.0.1:59491")
	if err != nil {
		t.Fatal(err)
	}
	m.TCPInventory = func(context.Context) ([]TCPBinding, error) { return nil, nil }
	standalone := n.ConfigDir + "-bulgaria"
	cfgPath := filepath.Join(standalone, "baft.yaml")
	if err := writeFile(cfgPath, []byte("installer-owned config\n"), 0600); err != nil {
		t.Fatal(err)
	}
	unit := fmt.Sprintf("# baft-managed: true\n[Service]\nExecStart=%s run --config %s\n", baftBin, cfgPath)
	if err := writeFile(m.unitPath(), []byte(unit), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := m.PrepareEX(context.Background(), "candidate", ExParams{PublicAddress: "127.0.0.1", Port: 59492, Target: "127.0.0.1:2443", RouteID: "r", StageConfig: true, VerifyResources: true}); err == nil {
		t.Fatal("standalone installer service was adopted")
	}
	if b, err := os.ReadFile(m.unitPath()); err != nil || string(b) != unit {
		t.Fatal("external unit changed")
	}
	if b, err := os.ReadFile(cfgPath); err != nil || string(b) != "installer-owned config\n" {
		t.Fatal("standalone config changed")
	}
	if _, err := os.Lstat(m.stage("candidate")); !os.IsNotExist(err) {
		t.Fatal("refused namespace acquired candidate state")
	}
	if len(n.host.calls) != 0 {
		t.Fatal("refused namespace operated the external service")
	}
}
