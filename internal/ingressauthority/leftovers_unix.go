//go:build unix

package ingressauthority

import (
	"os"
	"sort"
)

// FencedLeftoverV1 is a read-only description of a claim that survived a
// restart. It is evidence that the port stays reserved, never a statement that
// anything was applied or is safe to clean up.
type FencedLeftoverV1 struct {
	Binding BindingV1
	// State is empty for a plain pending reservation without lifecycle data.
	State RecoveryState
}

// InspectFencedLeftovers lists every durable pending reservation together with
// its lifecycle state (APPLYING_UNVERIFIED / NEEDS_RECONCILIATION). It changes
// nothing: the claims stay in the store and keep fencing their numeric ports.
func (s *ReservationStore) InspectFencedLeftovers() ([]FencedLeftoverV1, error) {
	var out []FencedLeftoverV1
	err := s.withLock(func(_ *os.File, root *os.Root) error {
		claims, err := loadPending(root)
		if err != nil {
			return err
		}
		records, err := readLifecycle(root)
		if err != nil {
			return err
		}
		for key, b := range claims {
			item := FencedLeftoverV1{Binding: b}
			if e, ok := records[key]; ok {
				item.State = e.State
			}
			out = append(out, item)
		}
		for key, e := range records {
			if _, ok := claims[key]; !ok {
				// Lifecycle data without a pending claim is itself unexplained.
				out = append(out, FencedLeftoverV1{Binding: e.Binding, State: e.State})
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Binding.Port < out[j].Binding.Port })
		return nil
	})
	return out, err
}
