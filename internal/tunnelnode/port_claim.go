package tunnelnode

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// ClaimMetricsPort creates an exclusive persistent marker for an instance.
// It does not bind a socket, prove runtime ownership, or authorize a tunnel.
// A claim intentionally survives process exit; reconciliation is required
// before an operator can release or reuse its port.
func (m *Manager) ClaimMetricsPort(port int) (string, error) {
	if m.Instance == "" {
		return "", errors.New("metrics port claims require an explicit instance")
	}
	if !validPort(port) {
		return "", errors.New("invalid metrics port")
	}
	root := filepath.Dir(filepath.Dir(m.ConfigDir))
	claims := filepath.Join(root, ".metrics-claims")
	if err := os.MkdirAll(claims, 0700); err != nil {
		return "", err
	}
	// Existing claim directories must be private and must not be symlinks.
	info, err := os.Lstat(claims)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return "", errors.New("metrics claims directory must be a private real directory")
	}
	path := filepath.Join(claims, strconv.Itoa(port))
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		return "", fmt.Errorf("metrics port %d already claimed", port)
	}
	if err != nil {
		return "", err
	}
	if _, err = f.WriteString(m.Instance + "\n"); err != nil {
		f.Close()
		return "", err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	// Sync the directory entry too: syncing the file alone does not ensure
	// the exclusive claim survives a power loss.
	dir, err := os.Open(claims)
	if err != nil {
		return "", err
	}
	err = dir.Sync()
	closeErr := dir.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	return path, nil
}
