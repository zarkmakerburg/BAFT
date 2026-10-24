//go:build linux

package ingressinventory

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// A stable later hash must never be attached to claims parsed before a write.
func TestInstalledSourceFingerprintRejectsParseHashDrift(t *testing.T) {
	for _, change := range []string{"claim", "same-claim-bytes", "add-unit", "add-dropin", "remove", "replace-file", "replace-root", "add-empty-dropin", "replace-empty-dropin"} {
		t.Run(change, func(t *testing.T) {
			_, root, _ := installedFixture(t)
			file := filepath.Join(root, "foreign.socket")
			if err := os.WriteFile(file, []byte("[Socket]\nListenStream=54619\n"), 0600); err != nil {
				t.Fatal(err)
			}
			dropdir := filepath.Join(root, "foreign.socket.d")
			var originalDrop os.FileInfo
			if change == "replace-empty-dropin" {
				if err := os.Mkdir(dropdir, 0700); err != nil {
					t.Fatal(err)
				}
				var err error
				originalDrop, err = os.Stat(dropdir)
				if err != nil {
					t.Fatal(err)
				}
			}
			observed, err := InspectSystemdSocketUnits(root)
			if err != nil {
				t.Fatal(err)
			}
			before, err := sourceFingerprint(observed)
			if err != nil || len(before.Files) != 1 || len(observed.Claims) != 1 || observed.Claims[0].Port != 54619 {
				t.Fatalf("unchanged protected source lost its parsed claim/provenance: %v %+v", err, before)
			}
			original, err := os.Stat(file)
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "add-empty-dropin":
				err = os.Mkdir(dropdir, 0700)
			case "replace-empty-dropin":
				err = os.Rename(dropdir, dropdir+".old")
				if err == nil {
					err = os.Mkdir(dropdir, 0700)
				}
				if err == nil {
					err = os.Chtimes(dropdir, originalDrop.ModTime(), originalDrop.ModTime())
				}
			case "claim":
				err = os.WriteFile(file, []byte("[Socket]\nListenStream=54620\n"), 0600)
			case "same-claim-bytes":
				err = os.WriteFile(file, []byte("# changed source\n[Socket]\nListenStream=54619\n"), 0600)
			case "add-unit":
				err = os.WriteFile(filepath.Join(root, "new.socket"), []byte("[Socket]\nListenStream=54620\n"), 0600)
			case "add-dropin":
				err = os.Mkdir(filepath.Join(root, "foreign.socket.d"), 0700)
				if err == nil {
					err = os.WriteFile(filepath.Join(root, "foreign.socket.d", "override.conf"), []byte("[Socket]\nListenStream=54620\n"), 0600)
				}
			case "remove":
				err = os.Remove(file)
			case "replace-file":
				err = os.Rename(file, file+".old")
				if err == nil {
					err = os.WriteFile(file, []byte("[Socket]\nListenStream=54619\n"), 0600)
				}
			case "replace-root":
				err = os.Rename(root, root+".old")
				if err == nil {
					err = os.Mkdir(root, 0700)
				}
				if err == nil {
					err = os.WriteFile(file, []byte("[Socket]\nListenStream=54619\n"), 0600)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			// Digest and opened identity checks must work even when a writer
			// restores the original timestamp (same-size claim change too).
			if change == "claim" || change == "same-claim-bytes" || change == "replace-file" {
				if err := os.Chtimes(file, original.ModTime(), original.ModTime()); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Chtimes(root, observed.parsedRoot.ModTime(), observed.parsedRoot.ModTime()); err != nil {
				t.Fatal(err)
			}
			if _, err := sourceFingerprint(observed); !errors.Is(err, ErrCoverageIncomplete) {
				t.Fatalf("parsed claims accepted with changed source %s: %v", change, err)
			}
		})
	}
}
