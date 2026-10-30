//go:build !unix

package agent

import (
	"errors"
	"io"
	"os"
)

func lockSeenJournal(string) (func(), error) {
	return nil, errors.New("seen-job process locking is unsupported on this platform")
}

func readSeenJournal(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxSeenJournalBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxSeenJournalBytes {
		return nil, errors.New("seen-job replay journal exceeds size limit")
	}
	return raw, nil
}

// Signed-job execution already fails closed when cross-process locking is unsupported.
func validateSeenStateDir(string) error { return nil }
