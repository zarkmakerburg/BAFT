package installer_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The agent decides once, at startup, whether operational public ingress is
// usable (installed managed-node policy + owned nft chain). install_agent must
// therefore provision both BEFORE it starts or restarts the agent, and must
// restart an already-running agent when a rerun changes them. Before v0.5.0
// the order was reversed: `install.sh --agent-only --provision-ingress-policy`
// left public ingress disabled until a manual restart (guided-flow e2e run
// 41255411266: "managed-node policy unsupported or untrusted").
func TestInstallAgentProvisionsIngressBeforeStartingAgent(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	start := strings.Index(src, "\ninstall_agent() {")
	if start < 0 {
		t.Fatal("install_agent() not found")
	}
	end := strings.Index(src[start+1:], "\n}\n")
	if end < 0 {
		t.Fatal("install_agent() has no end")
	}
	body := src[start : start+1+end]
	pos := func(re string) int {
		t.Helper()
		loc := regexp.MustCompile(re).FindStringIndex(body)
		if loc == nil {
			t.Fatalf("install_agent lacks %q", re)
		}
		return loc[0]
	}
	chain := pos(`(?m)^\s*install_ingress_chain$`)
	policy := pos(`(?m)^\s*provision_ingress_policy$`)
	restart := pos(`systemctl restart "\$BAFT_AGENT_UNIT\.service"`)
	startUnit := pos(`systemctl start "\$BAFT_AGENT_UNIT\.service"`)
	before := pos(`ingress_before="\$\(ingress_prereq_fingerprint\)"`)
	after := pos(`ingress_after="\$\(ingress_prereq_fingerprint\)"`)
	rerun := pos(`"\$ingress_before" != "\$ingress_after"`)
	if !(before < chain && before < policy && chain < after && policy < after) {
		t.Fatal("the ingress fingerprint must bracket chain and policy provisioning")
	}
	if !(after < rerun && rerun < restart && rerun < startUnit) {
		t.Fatal("a changed ingress prerequisite must be able to force the restart decision")
	}
	if !(chain < restart && policy < restart && chain < startUnit && policy < startUnit) {
		t.Fatal("ingress chain and policy must be provisioned before the agent is started or restarted")
	}
	if !strings.Contains(src, "\ningress_prereq_fingerprint() {") {
		t.Fatal("ingress_prereq_fingerprint() is missing")
	}
}
