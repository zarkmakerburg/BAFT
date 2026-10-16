//go:build !linux

package tunnelnode

import (
	"context"
	"errors"
)

func hostTCPInventory(context.Context) ([]TCPBinding, error) {
	return nil, errors.New("passive TCP resource inventory requires Linux procfs")
}

func serviceSocketInodes(context.Context, string) (map[uint64]bool, error) {
	return nil, errors.New("service socket ownership requires Linux procfs")
}

func hostUnixSocketInode(context.Context, string) (uint64, error) {
	return 0, errors.New("Unix socket identity requires Linux procfs")
}
