package ingressownership

import "github.com/zarkmakerburg/baft/internal/ingressinventory"

type SystemdSourcesSnapshot = ingressinventory.SystemdSourcesSnapshot

func InspectSystemdSocketSources(directories []string) (SystemdSourcesSnapshot, error) {
	return ingressinventory.InspectSystemdSocketSources(directories)
}
