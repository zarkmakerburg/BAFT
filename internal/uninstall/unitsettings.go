package uninstall

// Read access to the parameters of the units install.sh writes, for tools that
// must act on exactly the install a unit describes (the updater recovers the
// installer inputs from them). A unit is accepted only when it is
// byte-for-byte what the installer renders for the parameters it names, so a
// hand-edited or foreign unit is never trusted.

// ServiceUnitSettings are the parameters a transport unit names.
type ServiceUnitSettings = serviceParams

// AgentUnitSettings are the parameters an agent unit names.
type AgentUnitSettings = agentParams

// ParseInstallerServiceUnit returns the parameters of a transport unit that is
// exactly the installer's; ok is false for any other unit.
func ParseInstallerServiceUnit(raw string) (ServiceUnitSettings, bool) {
	p, ok := installerServiceParams(raw)
	return p, ok && renderServiceUnit(p) == raw
}

// ParseInstallerAgentUnit returns the parameters of an agent unit that is
// exactly the installer's; ok is false for any other unit.
func ParseInstallerAgentUnit(raw string) (AgentUnitSettings, bool) {
	p, ok := installerAgentParams(raw)
	return p, ok && renderAgentUnit(p) == raw
}

// InstallerAgentUnit is the agent unit install.sh writes (for tests and tools
// that need to recognise it).
func InstallerAgentUnit(p AgentUnitSettings) string { return renderAgentUnit(p) }
