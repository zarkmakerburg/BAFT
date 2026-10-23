//go:build unix

package ingressownership

import (
	"errors"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
	"os"
	"path/filepath"
	"testing"
)

func TestRestartPreparedManifestDoesNotAuthorizeSocket(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := ingressmanifest.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	spec := testSpec(t, "restart")
	proof, err := store.Prepare(spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, err = ingressmanifest.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runtime := NewRuntime()
	if err = runtime.RecoverPrepared(store, proof); !errors.Is(err, ErrUnverified) {
		t.Fatalf("restart accepted staged bytes: %v", err)
	}
	if len(runtime.sessions) != 0 {
		t.Fatal("restart started an unverified socket")
	}
}
