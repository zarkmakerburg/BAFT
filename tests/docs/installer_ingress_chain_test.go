package docs_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressownership"
)

// The installer's nft script must create exactly the layout that
// ingressownership.ReadNFTLabChainHandle pins (filter, input hook, priority
// -100, policy accept). Fixtures are real `nft -j list ruleset` output written
// by tests/e2e/installer_ingress_chain.sh --write-fixtures.
func TestInstallerIngressChainScriptMatchesPinnedLayout(t *testing.T) {
	b, err := os.ReadFile("../../install.sh")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`add chain inet baft ingress \{ ([^}]*) \}`).FindAllStringSubmatch(string(b), -1)
	if len(m) != 1 {
		t.Fatalf("install.sh must contain exactly one 'add chain inet baft ingress' script, found %d", len(m))
	}
	if got, want := strings.TrimSpace(m[0][1]), "type filter hook input priority -100; policy accept;"; got != want {
		t.Fatalf("installer chain spec %q, pinned layout is %q", got, want)
	}
	if !strings.Contains(string(b), "add table inet baft\\nadd chain inet baft ingress") {
		t.Fatal("installer script must create only table inet baft and the ingress chain")
	}
}

func TestReadNFTLabChainHandleAcceptsInstallerOutput(t *testing.T) {
	raw, err := os.ReadFile("testdata/installer_chain_ruleset.json")
	if err != nil {
		t.Fatal(err)
	}
	h, err := ingressownership.ReadNFTLabChainHandle(raw)
	if err != nil || h == 0 {
		t.Fatalf("installer-created chain rejected: handle=%d err=%v", h, err)
	}
}

func TestReadNFTLabChainHandleRefusesForeignLayoutFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/installer_chain_ruleset_foreign_layout.json")
	if err != nil {
		t.Fatal(err)
	}
	if h, err := ingressownership.ReadNFTLabChainHandle(raw); err == nil {
		t.Fatalf("foreign layout accepted, handle %d", h)
	}
}
