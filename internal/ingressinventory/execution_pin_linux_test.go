//go:build linux

package ingressinventory

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestManagedExecutionPinStableAndTamperedLAB(t *testing.T) {
	ns, err := namespaceIdentity()
	if err != nil {
		t.Fatal(err)
	}
	initNS, err := os.Readlink("/proc/1/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	if ns == initNS {
		t.Skip("requires isolated Linux LAB")
	}
	path, _, policy := installedFixture(t)
	writeProfileFixture(t, path, policy)
	ctx := context.Background()
	pin, err := pinInstalledManagedLABExecution(ctx, path, policy.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if pin.LAB.ProcessStartTicks == 0 || pin.LAB.ExecutableInode == 0 ||
		len(pin.LAB.ExecutableSHA256) != 64 || pin.LAB.MountNS == "" ||
		pin.LAB.CgroupNS == "" || pin.LAB.CgroupMembership == "" {
		t.Fatalf("incomplete kernel execution baseline: %+v", pin.LAB)
	}
	if err := checkInstalledManagedLABAtEffect(ctx, path, policy.NodeID, pin); err != nil {
		t.Fatalf("stable remeasurement rejected: %v", err)
	}
	cases := []struct {
		name string
		edit func(*ManagedLABExecutionPin)
	}{
		{"process", func(p *ManagedLABExecutionPin) { p.LAB.ProcessStartTicks++ }},
		{"binary", func(p *ManagedLABExecutionPin) { p.LAB.ExecutableSHA256 = strings.Repeat("0", 64) }},
		{"binary-inode", func(p *ManagedLABExecutionPin) { p.LAB.ExecutableInode++ }},
		{"netns", func(p *ManagedLABExecutionPin) { p.LAB.CurrentNetNS = "net:[0]" }},
		{"mount", func(p *ManagedLABExecutionPin) { p.LAB.MountNS = "mnt:[0]" }},
		{"cgroup", func(p *ManagedLABExecutionPin) { p.LAB.CgroupMembership = "0::/other\n" }},
		{"profile", func(p *ManagedLABExecutionPin) { p.Policy.ProfileSHA256 = strings.Repeat("0", 64) }},
		{"source", func(p *ManagedLABExecutionPin) { p.Sources[0].SHA256 = strings.Repeat("0", 64) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			modified := pin
			modified.Sources = append([]SourceIdentity(nil), pin.Sources...)
			tc.edit(&modified)
			if err := checkInstalledManagedLABAtEffect(ctx, path, policy.NodeID, modified); !errors.Is(err, ErrCoverageIncomplete) {
				t.Fatalf("mutated pin was treated as authority: %v", err)
			}
		})
	}
}
