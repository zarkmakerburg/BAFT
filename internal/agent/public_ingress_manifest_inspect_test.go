package agent

import (
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectPreparedIngressManifestNeverClaimsHostOwnership(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := ingressmanifest.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	spec := ingressmanifest.Spec{TunnelID: "tun-a", Generation: 1, Config: ingressmanifest.Config{Listen: "0.0.0.0:54619", Target: "127.0.0.1:1443"}}
	expected, err := ingressmanifest.Preview(spec)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inspectPreparedIngressManifest(store, expected); err == nil {
		t.Fatal("absent manifest accepted")
	}
	written, err := store.Prepare(spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := inspectPreparedIngressManifest(store, written)
	if err != nil {
		t.Fatal(err)
	}
	if !verified.Prepared || !verified.BytesVerified || verified.LiveOwnershipVerified || verified.ReadyToApply {
		t.Fatalf("invalid host ownership assertion: %+v", verified)
	}
	wrong := written
	wrong.Generation++
	if _, err := inspectPreparedIngressManifest(store, wrong); err == nil {
		t.Fatal("stale proof accepted")
	}
}
