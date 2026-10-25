package main

// Menu "Update" (v0.4.0): read-only guidance. It shows the installed signed
// release and the exact, verified ways to move to another one. It changes
// nothing: updates are applied by the verified installer or by a signed agent
// job planned in the BCC Update Center, never by this menu.
func (m *menu) updateGuide() {
	if rs, err := readReleaseState(m.cfg.releaseState); err == nil {
		m.printf("Installed signed release: %s (commit %s)\n", clean(rs.Version), clean(rs.Commit))
	} else {
		m.printf("Installed release: unknown (no release state; source install?)\n")
	}
	m.printf("\nThis menu never updates anything. To update, use one of:\n")
	m.printf("  1. BCC Update Center (Updates page): import a signed bundle, see every server's\n")
	m.printf("     version, review a plan, then queue the signed update job for ONE server.\n")
	m.printf("  2. The verified installer, with a signed offline bundle (no internet needed):\n")
	m.printf("       tar -xzf baft-offline-vX.Y.Z.tar.gz && cd baft-offline-vX.Y.Z\n")
	m.printf("       sudo bash install.sh --offline . --plan     # preview, changes nothing\n")
	m.printf("       sudo bash install.sh --offline . --yes      # apply, rolls back by itself on failure\n")
	m.printf("\nRollback: the installer restores the previous files if an update fails. To go back to an\n")
	m.printf("older release on purpose, use that release's bundle with --allow-downgrade (the installer\n")
	m.printf("refuses downgrades otherwise). Use the same role flags as the original install.\n")
	m.printf("Nothing was changed.\n")
}
