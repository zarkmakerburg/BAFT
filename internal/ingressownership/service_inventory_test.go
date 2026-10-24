package ingressownership

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Some shared/virtual filesystems create Go TempDir with group write bits.
// Tests explicitly establish the production scanner's trusted-source mode.
func trustedInventoryFixture(t *testing.T) string {
	t.Helper()
	dir := privateInventoryTestDir(t)
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func privateInventoryTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Colima's Linux VM test process can inherit a permissive umask or
	// default ACL; fixtures must explicitly meet the exact trust rule.
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeInventoryFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestStoppedSystemdSocketInventoryProtectsForeignDynamicPorts(t *testing.T) {
	dir := trustedInventoryFixture(t)
	writeInventoryFixture(t, dir, "foreign.socket",
		"[Unit]\nDescription=Foreign stopped service\n[Socket]\nListenStream=54619\nListenStream=[::]:443\nListenStream=/run/foreign.sock\n[Install]\nWantedBy=sockets.target\n")
	// This fixture models a disabled/stopped unit; no process is bound and
	// there is no active socket to discover using /proc or net.Listen.
	inventory, err := InspectSystemdSocketUnits(dir)
	if err != nil || !inventory.ScopedComplete {
		t.Fatalf("stopped foreign socket claim not observed: %+v %v", inventory, err)
	}
	if len(inventory.Claims) != 2 {
		t.Fatalf("wrong stopped persistent TCP claims: %+v", inventory.Claims)
	}
	for _, port := range []uint16{443, 54619} {
		claimed, err := inventory.ReservedInThisSystemdSource(port)
		if err != nil || !claimed {
			t.Fatalf("stopped foreign %d unprotected: %v %v", port, claimed, err)
		}
	}
	if claimed, err := inventory.ReservedInThisSystemdSource(2053); err != nil || claimed {
		t.Fatalf("arbitrary port became permanently forbidden: %v %v", claimed, err)
	}
	again, err := InspectSystemdSocketUnits(dir)
	if err != nil || !reflect.DeepEqual(inventory, again) {
		t.Fatalf("inventory unstable without host mutations: %v %+v %+v", err, inventory, again)
	}
}

func TestStoppedSystemdSocketOverridesReplacePersistedReservations(t *testing.T) {
	dir := trustedInventoryFixture(t)
	writeInventoryFixture(t, dir, "alpha.socket", "[Socket]\nListenStream=54619\n")
	override := filepath.Join(dir, "alpha.socket.d")
	if err := os.Mkdir(override, 0700); err != nil {
		t.Fatal(err)
	}
	writeInventoryFixture(t, override, "10-custom.conf", "[Socket]\nListenStream=\nListenStream=2053\n")
	writeInventoryFixture(t, override, "20-other.conf", "[Socket]\nListenStream=127.0.0.1:443\n")
	result, err := InspectSystemdSocketUnits(dir)
	if err != nil {
		t.Fatal(err)
	}
	for port, expected := range map[uint16]bool{54619: false, 2053: true, 443: true} {
		got, err := result.ReservedInThisSystemdSource(port)
		if err != nil || got != expected {
			t.Fatalf("port %d wrong claim after systemd drop-in: got=%v want=%v err=%v", port, got, expected, err)
		}
	}
	if result.Claims[0].Source != "alpha.socket.d/20-other.conf" &&
		result.Claims[0].Source != "alpha.socket.d/10-custom.conf" {
		t.Fatalf("missing exact configuration source: %+v", result.Claims)
	}
}

func TestStoppedSystemdSocketIncompleteSourcesRefuseEmptySuccess(t *testing.T) {
	for _, tc := range []struct {
		name, filename, body string
	}{
		{"unknown-port-name", "foreign.socket", "[Socket]\nListenStream=https\n"},
		{"unparseable-IPv6", "foreign.socket", "[Socket]\nListenStream=::1:54619\n"},
		{"invalid-port", "foreign.socket", "[Socket]\nListenStream=65536\n"},
		{"zero-port", "foreign.socket", "[Socket]\nListenStream=0\n"},
		{"malformed-unit", "foreign.socket", "[Socket]\nListenStream\n"},
		{"unterminated-section", "foreign.socket", "[Socket\nListenStream=54619\n"},
		{"nul-in-unit", "foreign.socket", "[Socket]\nListenStream=54619\x00\n"},
		{"orphan-dropins", "alone.socket.d", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := trustedInventoryFixture(t)
			if tc.name == "orphan-dropins" {
				if err := os.Mkdir(filepath.Join(dir, tc.filename), 0700); err != nil {
					t.Fatal(err)
				}
			} else {
				writeInventoryFixture(t, dir, tc.filename, tc.body)
			}
			inventory, err := InspectSystemdSocketUnits(dir)
			if !errors.Is(err, ErrPersistentServiceInventoryIncomplete) || inventory.ScopedComplete {
				t.Fatalf("incomplete inventory was treated as trusted: %v %+v", err, inventory)
			}
			if allowed, err := inventory.ReservedInThisSystemdSource(54619); allowed || !errors.Is(err, ErrPersistentServiceInventoryIncomplete) {
				t.Fatalf("incomplete inventory said port 54619 safe: %v %v", allowed, err)
			}
		})
	}
}

func TestStoppedSystemdSocketSymlinkAndWritableSourcesFailClosed(t *testing.T) {
	for _, tc := range []string{"unit-symlink", "dropin-symlink", "writable-source", "writable-unit"} {
		t.Run(tc, func(t *testing.T) {
			dir := trustedInventoryFixture(t)
			unit := filepath.Join(dir, "foreign.socket")
			writeInventoryFixture(t, dir, "foreign.socket", "[Socket]\nListenStream=54619\n")
			switch tc {
			case "unit-symlink":
				if err := os.Remove(unit); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(dir, "real.socket"), unit); err != nil {
					t.Fatal(err)
				}
			case "dropin-symlink":
				if err := os.Symlink(filepath.Join(dir, "missing"), filepath.Join(dir, "foreign.socket.d")); err != nil {
					t.Fatal(err)
				}
			case "writable-source":
				if err := os.Chmod(dir, 0777); err != nil {
					t.Fatal(err)
				}
			case "writable-unit":
				if err := os.Chmod(unit, 0666); err != nil {
					t.Fatal(err)
				}
			}
			got, err := InspectSystemdSocketUnits(dir)
			if !errors.Is(err, ErrPersistentServiceInventoryIncomplete) || got.ScopedComplete {
				t.Fatalf("untrusted systemd source accepted: %+v err=%v", got, err)
			}
		})
	}
}

func TestStoppedSystemdSocketGlobalAuthorityIsNotInferred(t *testing.T) {
	empty, err := InspectSystemdSocketUnits(trustedInventoryFixture(t))
	if err != nil || !empty.ScopedComplete {
		t.Fatalf("empty scoped inventory unexpected: %+v %v", empty, err)
	}
	// A complete single-root socket-unit scan is NOT a complete inventory
	// of vendor dirs, .service direct binds, containers or other registries.
	// No operational admission API is exposed on these scoped observations.
}

func TestStoppedServiceInventoryVetoNeverOpensLiveOrUnverifiedPort(t *testing.T) {
	dir := trustedInventoryFixture(t)
	writeInventoryFixture(t, dir, "imperial.socket", "[Socket]\nListenStream=54619\n")
	inventory, err := InspectSystemdSocketUnits(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := inventory.RefuseAdmissionFromSocketInventory(54619); !errors.Is(err, ErrConflict) {
		t.Fatalf("known persistent stopped service was not vetoed: %v", err)
	}
	if err := inventory.RefuseAdmissionFromSocketInventory(2053); !errors.Is(err, ErrPersistentServiceInventoryIncomplete) {
		t.Fatalf("empty single-source inventory was promoted to global port permission: %v", err)
	}
	// A caller cannot make incomplete inventory an allow-all by clearing a
	// flag or providing an empty result. Both variants deny all admission.
	if err := (SystemdSocketInventory{}).RefuseAdmissionFromSocketInventory(2053); !errors.Is(err, ErrPersistentServiceInventoryIncomplete) {
		t.Fatalf("zero-value inventory permitted host change: %v", err)
	}
	if err := inventory.RefuseAdmissionFromSocketInventory(0); !errors.Is(err, ErrPersistentServiceInventoryIncomplete) {
		t.Fatalf("invalid numeric port escaped fail-closed guard: %v", err)
	}
}
