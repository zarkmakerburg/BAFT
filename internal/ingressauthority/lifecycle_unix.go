//go:build unix

package ingressauthority

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"syscall"
)

type RecoveryState string

const (
	StateNeedsReconciliation RecoveryState = "NEEDS_RECONCILIATION"
	StateApplying            RecoveryState = "APPLYING_UNVERIFIED"
)
const lifecycleFile = "lifecycle.json"

// Lifecycle entries record incomplete effects only. Neither a journal value
// nor PendingGuard authorizes applying, releasing, or minting a live Lease.
type lifecycleEntry struct {
	Binding BindingV1                  `json:"binding"`
	State   RecoveryState              `json:"state"`
	Policy  *InstalledPolicyIdentityV1 `json:"installed_policy,omitempty"`
}

func readLifecycle(root *os.Root) (map[string]lifecycleEntry, error) {
	result := make(map[string]lifecycleEntry)
	f, err := root.OpenFile(lifecycleFile, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return nil, ErrUnverified
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	owner, ok := stat.Sys().(*syscall.Stat_t)
	if !ok || !stat.Mode().IsRegular() || stat.Mode().Perm() != 0600 || owner.Nlink != 1 ||
		int(owner.Uid) != os.Geteuid() || stat.Size() < 2 || stat.Size() > reservationMaxBytes {
		return nil, ErrUnverified
	}
	raw, err := io.ReadAll(io.LimitReader(f, reservationMaxBytes+1))
	if err != nil || len(raw) > reservationMaxBytes {
		return nil, ErrUnverified
	}
	if json.Unmarshal(raw, &result) != nil || result == nil {
		return nil, ErrUnverified
	}
	canonical, err := json.Marshal(result)
	if err != nil || !bytes.Equal(canonical, raw) {
		return nil, ErrUnverified
	}
	for key, value := range result {
		if value.Binding.Validate() != nil || key != reservationKey(value.Binding) ||
			value.State != StateApplying && value.State != StateNeedsReconciliation {
			return nil, ErrUnverified
		}
		if value.Policy != nil && !value.Policy.validFor(value.Binding) {
			return nil, ErrUnverified
		}
	}
	return result, nil
}

func writeLifecycle(fd *os.File, root *os.Root, entries map[string]lifecycleEntry) error {
	raw, err := json.Marshal(entries)
	if err != nil || len(raw) > reservationMaxBytes {
		return ErrUnverified
	}
	f, err := root.OpenFile(".lifecycle-write", os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(".lifecycle-write")
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = root.Rename(".lifecycle-write", lifecycleFile); err != nil {
		return err
	}
	return fd.Sync()
}

// MarkApplying records possible partial effects durably before a future
// executor is allowed to try any host mutation. The only subsequent safe
// restart interpretation is NEEDS_RECONCILIATION.
func (g *PendingGuard) MarkApplying(ctx context.Context, b BindingV1) error {
	if g == nil {
		return ErrUnverified
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if ctx == nil || ctx.Err() != nil || g.fd == nil || g.root == nil || g.store == nil || b != g.binding {
		return ErrUnverified
	}
	if err := checkAuthorityRoot(g.fd, g.root, g.store.dir, g.store.identity); err != nil {
		return err
	}
	claims, err := loadPending(g.root)
	if err != nil {
		return err
	}
	if old, ok := claims[reservationKey(b)]; !ok || old != b {
		return ErrUnverified
	}
	entries, err := readLifecycle(g.root)
	if err != nil {
		return err
	}
	if old, exists := entries[reservationKey(b)]; exists && old.Binding != b {
		return ErrUnverified
	}
	entry := entries[reservationKey(b)]
	entry.Binding, entry.State = b, StateApplying
	entries[reservationKey(b)] = entry
	return writeLifecycle(g.fd, g.root, entries)
}

// InspectRecovery never interprets stopped process, missing lifecycle data,
// lock loss, or partial apply as port availability.
func (s *ReservationStore) InspectRecovery(b BindingV1) (RecoveryState, error) {
	if err := b.Validate(); err != nil {
		return "", err
	}
	var state RecoveryState
	err := s.withLock(func(_ *os.File, root *os.Root) error {
		claims, err := loadPending(root)
		if err != nil {
			return err
		}
		if current, ok := claims[reservationKey(b)]; !ok || current != b {
			return ErrUnverified
		}
		records, err := readLifecycle(root)
		if err != nil {
			return err
		}
		if existing, ok := records[reservationKey(b)]; ok && existing.Binding != b {
			return ErrUnverified
		}
		state = StateNeedsReconciliation
		return nil
	})
	return state, err
}
