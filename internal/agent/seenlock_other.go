//go:build !unix

package agent

import (
	"errors"
	"os"
)

func lockSeenJournal(string) (func(), error) {
	return nil, errors.New("seen-job process locking is unsupported on this platform")
}

func readSeenJournal(path string) ([]byte, error) { return os.ReadFile(path) }
