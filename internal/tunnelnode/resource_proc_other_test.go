//go:build !linux

package tunnelnode

import "testing"

func requireProcessResourceEvidence(t *testing.T) {
	t.Helper()
	t.Skip("process resource evidence requires Linux")
}
