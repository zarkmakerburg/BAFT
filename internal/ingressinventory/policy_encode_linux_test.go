//go:build linux

package ingressinventory

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestManagedPolicyEncoderUsesTheInstalledWireContract(t *testing.T) {
	ns, err := namespaceIdentity()
	if err != nil {
		t.Fatal(err)
	}
	p := InstalledManagedNodePolicyV1{SchemaVersion: 1, InventoryVersion: ManagedCoverageVersion, NodeID: "ir-policy", Platform: "linux", NetworkNamespace: ns, Transport: "dedicated", Manager: "systemd-socket-v1", SourceRoots: []string{"/etc"}}
	raw, err := MarshalInstalledManagedNodePolicyV1(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := parseInstalledPolicy(raw, p.NodeID, ns)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.NodeID != p.NodeID || decoded.NetNS != ns || !reflect.DeepEqual(decoded.SourceRoots, p.SourceRoots) {
		t.Fatal("installer encoder drifted from sole installed decoder")
	}
	for _, mutate := range []func(*InstalledManagedNodePolicyV1){
		func(p *InstalledManagedNodePolicyV1) { p.NetworkNamespace = "net:[0]" },
		func(p *InstalledManagedNodePolicyV1) { p.InventoryVersion++ },
		func(p *InstalledManagedNodePolicyV1) { p.NodeID = strings.Repeat("n", 32<<10) },
		func(p *InstalledManagedNodePolicyV1) { p.Manager = "unknown" },
		func(p *InstalledManagedNodePolicyV1) { p.SourceRoots = []string{"/tmp"} },
		func(p *InstalledManagedNodePolicyV1) { p.SourceRoots = []string{"/etc", "/etc"} },
	} {
		bad := p
		mutate(&bad)
		if _, err := MarshalInstalledManagedNodePolicyV1(context.Background(), bad); err == nil {
			t.Fatal("unsupported/untrusted installer policy encoded")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := MarshalInstalledManagedNodePolicyV1(ctx, p); !errors.Is(err, context.Canceled) {
		t.Fatalf("encoder ignored canceled context: %v", err)
	}
}
