//go:build unix

package ingressinventory

import (
	"os"
	"syscall"
)

// stableSourceIdentity deliberately excludes access time and other volatile
// FileInfo.Sys fields: observing a source must not change its identity.
func stableSourceIdentity(info os.FileInfo) (sourceObjectIdentity, error) {
	if info == nil {
		return sourceObjectIdentity{}, ErrPersistentServiceInventoryIncomplete
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return sourceObjectIdentity{}, ErrPersistentServiceInventoryIncomplete
	}
	return sourceObjectIdentity{device: uint64(stat.Dev), inode: uint64(stat.Ino), mode: info.Mode(), size: info.Size(), modUnixNS: info.ModTime().UnixNano()}, nil
}
