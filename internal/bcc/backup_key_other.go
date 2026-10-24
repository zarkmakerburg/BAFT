//go:build !unix

package bcc

import (
	"errors"
	"os"
)

// Until ownership and non-following opens can be verified on this platform,
// refuse a file-backed backup key instead of trusting an unsafe alias.
func openTrustedBackupKey(string) (*os.File, error) {
	return nil, errors.New("secure backup key file validation unsupported on this platform")
}
