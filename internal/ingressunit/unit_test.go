package ingressunit

import (
	"strings"
	"testing"

	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

func candidateSpec() ingressmanifest.Spec {
	return ingressmanifest.Spec{TunnelID: "tun-a", InstanceID: "node-a", Generation: 1, Config: ingressmanifest.Config{
		Listen: "0.0.0.0:54619", Target: "127.0.0.1:1443", MaxClients: 128, DialTimeoutMS: 3000,
	}}
}

func TestRenderRequiresExactManifestAndDoesNotAdoptChangedUnit(t *testing.T) {
	spec := candidateSpec()
	proof, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	unit, err := Render(spec, proof, "listener-1", "/usr/local/bin/baft", "baft")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(unit.Name, "baft-ingress-") || !strings.Contains(string(unit.Bytes), "--listen 0.0.0.0:54619 --target 127.0.0.1:1443") || !strings.Contains(string(unit.Bytes), "--allow-public") || !strings.Contains(string(unit.Bytes), proof.ManifestSHA256) || len(unit.SHA256) != 64 {
		t.Fatalf("unexpected unit candidate: %+v", unit)
	}
	if !MatchesOwnedBytes(unit, unit.Bytes) || MatchesOwnedBytes(unit, append(append([]byte{}, unit.Bytes...), '\n')) {
		t.Fatal("unit ownership accepted modified bytes")
	}
	if same, err := Render(spec, proof, "listener-1", "/usr/local/bin/baft", "baft"); err != nil || same.Name != unit.Name || same.SHA256 != unit.SHA256 {
		t.Fatalf("candidate is not deterministic: %+v %v", same, err)
	}
	other, err := Render(spec, proof, "listener-2", "/usr/local/bin/baft", "baft")
	if err != nil {
		t.Fatal(err)
	}
	if other.Name == unit.Name {
		t.Fatal("distinct listeners share an owned systemd unit filename")
	}
	if other.SHA256 == unit.SHA256 {
		t.Fatal("distinct listener candidates must have different content")
	}
	changed := spec
	changed.Config.Target = "127.0.0.1:1444"
	changedProof, err := ingressmanifest.Preview(changed)
	if err != nil {
		t.Fatal(err)
	}
	changedUnit, err := Render(changed, changedProof, "listener-1", "/usr/local/bin/baft", "baft")
	if err != nil {
		t.Fatal(err)
	}
	if changedUnit.Name == unit.Name {
		t.Fatal("different manifest replaced same-generation owned listener path")
	}
	wrong := proof
	wrong.ManifestSHA256 = strings.Repeat("a", 64)
	if _, err := Render(spec, wrong, "listener-1", "/usr/local/bin/baft", "baft"); err == nil {
		t.Fatal("wrong manifest proof accepted")
	}
	for _, tc := range []struct{ listener, bin, user string }{
		{"../other", "/usr/local/bin/baft", "baft"},
		{"listener-1", "/usr/local/bin/baft %n", "baft"},
		{"listener-1", "baft", "baft"},
		{"listener-1", "/usr/local/bin/baft", "root\nExecStart=/bin/sh"},
		{"listener-1", "/usr/local/bin/baft", "root"},
	} {
		if _, err := Render(spec, proof, tc.listener, tc.bin, tc.user); err == nil {
			t.Fatalf("unsafe unit input accepted: %+v", tc)
		}
	}
}

func TestRenderRejectsUnsafeManifestIdentityInSystemdMetadata(t *testing.T) {
	for _, id := range []string{"bad\n[Service]\nExecStart=/bin/false", "bad\runit", "bad\x00id"} {
		spec := candidateSpec()
		spec.TunnelID = id
		proof, err := ingressmanifest.Preview(spec)
		if err != nil {
			continue
		} // The manifest validator may reject it sooner.
		if _, err := Render(spec, proof, "listener-1", "/usr/local/bin/baft", "baft"); err == nil {
			t.Fatalf("accepted unsafe tunnel identity %q", id)
		}
	}
}
