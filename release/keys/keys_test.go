package keys

import (
	"bytes"
	"testing"

	"github.com/zarkmakerburg/baft/internal/release"
)

func TestRootIsTheCommittedKey(t *testing.T) {
	got, err := Root()
	if err != nil {
		t.Fatal(err)
	}
	want, err := release.ReadPublic("root.pub")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("embedded root key differs from release/keys/root.pub")
	}
	if release.KeyID(got) == "" {
		t.Fatal("root key has no id")
	}
}
