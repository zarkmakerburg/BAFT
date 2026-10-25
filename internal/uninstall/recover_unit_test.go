package uninstall

import (
	"fmt"
	"strings"
	"testing"
)

const recoverUnitBody = "# baft-managed: true\n# baft-component: recover\n[Unit]\nDescription=BAFT boot recovery\nDefaultDependencies=no\nBefore=sysinit.target\n[Service]\nType=oneshot\nRemainAfterExit=yes\nExecStart=%s recover boot --config-dir /etc/baft\n[Install]\nWantedBy=sysinit.target\n"

// The unit install.sh writes is removed with the services (and stops pinning
// the baft binary); a look-alike is never claimed and keeps the binary.
func TestUninstallRemovesOwnedRecoverUnitOnly(t *testing.T) {
	r := newRig(t)
	r.installerEX(true)
	r.agent(true)
	r.write("units/baft-recover.service", fmt.Sprintf(recoverUnitBody, r.p("bin/baft")), 0o644)
	r.sys.set("baft-recover.service", "active", "enabled")
	p := r.plan(yes(Options{}))
	if has(p.Remove, r.p("units/baft-recover.service")) == nil {
		t.Fatalf("owned recover unit not planned for removal: %v", paths(p.Remove))
	}
	if _, err := r.env.Apply(ctx, p); err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"units/baft-recover.service", "bin/baft"} {
		if exists(r.p(gone)) {
			t.Errorf("%s still there", gone)
		}
	}
	if a, e := r.sys.state("baft-recover.service"); a != "inactive" || e != "disabled" {
		t.Fatalf("recover unit %s/%s", a, e)
	}
}

func TestUninstallNeverClaimsALookAlikeRecoverUnit(t *testing.T) {
	for name, body := range map[string]string{
		"no-label":      "[Service]\nExecStart=" + "%s" + " recover boot\n",
		"other-program": "# baft-managed: true\n# baft-component: recover\n[Service]\nExecStart=/bin/true recover boot\n",
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			r.installerEX(true)
			r.agent(true)
			unit := body
			if strings.Contains(body, "%s") {
				unit = fmt.Sprintf(body, r.p("bin/baft"))
			}
			r.write("units/baft-recover.service", unit, 0o644)
			p := r.plan(yes(Options{}))
			if has(p.Remove, r.p("units/baft-recover.service")) != nil {
				t.Fatal("a look-alike unit was planned for removal")
			}
			if _, err := r.env.Apply(ctx, p); err != nil {
				t.Fatal(err)
			}
			if !exists(r.p("units/baft-recover.service")) {
				t.Fatal("foreign unit was removed")
			}
		})
	}
}
