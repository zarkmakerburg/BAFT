package ingressownership

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
)

// SystemdSourcesSnapshot is an explicitly bounded collection of inspected
// systemd socket unit trees. It cannot prove global host completeness: units
// may also come from generators, templates, runtime state and other managers.
type SystemdSourcesSnapshot struct {
	Sources        []SystemdSocketInventory
	Claims         []PersistentTCPPortClaim
	ScopedComplete bool
}

// InspectSystemdSocketSources reads every explicitly configured systemd
// source root, preserving stopped foreign claims even when no TCP socket is
// currently listening. Missing, duplicate, malformed or replaced roots deny
// the entire scoped observation; an empty root list is NEVER treated as an
// empty successful inventory.
func InspectSystemdSocketSources(directories []string) (SystemdSourcesSnapshot, error) {
	var out SystemdSourcesSnapshot
	if len(directories) == 0 || len(directories) > 32 {
		return out, ErrPersistentServiceInventoryIncomplete
	}
	seen := make(map[string]struct{}, len(directories))
	for _, dir := range directories {
		if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
			return SystemdSourcesSnapshot{}, fmt.Errorf("%w: noncanonical source root", ErrPersistentServiceInventoryIncomplete)
		}
		if _, ok := seen[dir]; ok {
			return SystemdSourcesSnapshot{}, fmt.Errorf("%w: duplicate source root", ErrPersistentServiceInventoryIncomplete)
		}
		seen[dir] = struct{}{}
		observation, err := InspectSystemdSocketUnits(dir)
		if err != nil || !observation.ScopedComplete {
			return SystemdSourcesSnapshot{}, errors.Join(ErrPersistentServiceInventoryIncomplete, err)
		}
		out.Sources = append(out.Sources, observation)
		out.Claims = append(out.Claims, observation.Claims...)
	}
	sort.Slice(out.Claims, func(i, j int) bool {
		if out.Claims[i].Port != out.Claims[j].Port {
			return out.Claims[i].Port < out.Claims[j].Port
		}
		if out.Claims[i].Unit != out.Claims[j].Unit {
			return out.Claims[i].Unit < out.Claims[j].Unit
		}
		return out.Claims[i].Source < out.Claims[j].Source
	})
	out.ScopedComplete = true
	return out, nil
}

// VetoKnownStoppedPort never asserts global host availability. A confirmed
// stopped owner vetoes with ErrConflict; absence in the bounded source scan
// returns ErrPersistentServiceInventoryIncomplete, requiring independent
// inventory categories and Alpha's durable held authority before any bind
// or firewall effect. No nil success from this read-only adapter.
func (s SystemdSourcesSnapshot) VetoKnownStoppedPort(port uint16) error {
	if !s.ScopedComplete || len(s.Sources) == 0 || port == 0 {
		return ErrPersistentServiceInventoryIncomplete
	}
	for _, source := range s.Sources {
		if !source.ScopedComplete {
			return ErrPersistentServiceInventoryIncomplete
		}
	}
	for _, claim := range s.Claims {
		if claim.Port == port {
			return ErrConflict
		}
	}
	return ErrPersistentServiceInventoryIncomplete
}
