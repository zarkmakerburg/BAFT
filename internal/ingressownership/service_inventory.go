package ingressownership

import "github.com/zarkmakerburg/baft/internal/ingressinventory"

// Compatibility aliases preserve the current ingressownership surface while
// the authoritative bounded scanner lives in a dependency-leaf package that
// Alpha's ingressauthority can import without a package cycle.
var ErrPersistentServiceInventoryIncomplete = ingressinventory.ErrPersistentServiceInventoryIncomplete

type PersistentTCPPortClaim = ingressinventory.PersistentTCPPortClaim
type SystemdSocketInventory = ingressinventory.SystemdSocketInventory

func InspectSystemdSocketUnits(directory string) (SystemdSocketInventory, error) {
	return ingressinventory.InspectSystemdSocketUnits(directory)
}
