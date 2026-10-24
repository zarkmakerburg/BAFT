//go:build linux

package ingressinventory

import (
	"os"
	"testing"
)

func TestProtectedSourceRootDescriptorsAreReleased(t *testing.T) {
	count := func() int {
		t.Helper()
		entries, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Fatal(err)
		}
		return len(entries)
	}
	before := count()
	for i := 0; i < 32; i++ {
		if _, _, err := protectedSourceRootIdentity("/etc"); err != nil {
			t.Fatal(err)
		}
	}
	if after := count(); after > before+2 {
		t.Fatalf("source ancestry inspection leaked directory descriptors: before=%d after=%d", before, after)
	}
}
