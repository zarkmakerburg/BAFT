//go:build !unix

package ingressinventory

import "os"

// Without a supported stable device/inode observation, no source identity
// can be established. Never substitute path strings or volatile reflection.
func stableSourceIdentity(os.FileInfo) (sourceObjectIdentity, error) {
	return sourceObjectIdentity{}, ErrPersistentServiceInventoryIncomplete
}
