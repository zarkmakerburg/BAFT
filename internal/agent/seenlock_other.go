//go:build !unix

package agent

import "errors"

func lockSeenJournal(string) (func(), error) {
	return nil, errors.New("seen-job process locking is unsupported on this platform")
}
