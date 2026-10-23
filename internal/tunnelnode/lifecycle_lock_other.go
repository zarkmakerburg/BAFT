//go:build !unix

package tunnelnode

import (
	"context"
	"errors"
	"os"
)

func lockLifecycleFile(context.Context, string, os.FileInfo) (func(), error) {
	return nil, errors.New("tunnel lifecycle process locking is unsupported on this platform")
}
