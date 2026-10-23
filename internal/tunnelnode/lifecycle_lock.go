package tunnelnode

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// ErrLifecycleBusy means another cooperating process owns this instance's operation lock.
var ErrLifecycleBusy = errors.New("tunnel lifecycle is busy")

// lockLifecycle coordinates cooperating CLI/Agent processes in the trusted
// installer-provisioned instance StateDir. Durable records remain the recovery
// fence after a crash; acquiring a lock never grants live-resource ownership.
func (m *Manager) lockLifecycle(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(m.StateDir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(m.StateDir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0022 != 0 {
		return nil, errors.New("lifecycle StateDir must be a real directory without group/other write permission")
	}
	return lockLifecycleFile(ctx, filepath.Join(m.StateDir, ".lifecycle.lock"), info)
}
