//go:build unix

package ingressauthority

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"syscall"

	"github.com/zarkmakerburg/baft/internal/ingressinventory"
	"github.com/zarkmakerburg/baft/internal/ingressmanifest"
)

// Release of a numeric-port reservation.
//
// A reservation is released ONLY as the final step of a verified retirement,
// by the holder of the PendingGuard (OS lock) that fences the resource:
//
//   - the lifecycle records a CONSUMED lease (RestartAuthority rules: leased
//     intent matching, not superseded, installed policy pinned);
//   - the caller presents RetirementEvidenceV1 for exactly this binding
//     (journal retired, owner rule read back absent);
//   - the guard itself re-reads the kernel: complete protected coverage, no
//     stopped service claim, no LISTEN on the port and no inode-bearing socket
//     of the agent's uid in any state. A foreign listener blocks the release.
//
// The release is monotonic and durable: released.json keeps, per (node,port),
// the released generation and every spent JobID/OperationID/LeaseID/plan hash.
// A later reservation for the port needs a STRICTLY GREATER generation and
// fresh operation, lease and plan identities; a spent JobID can never be
// recorded again by any later operation. Nothing releases on a timer, from a
// missing map entry or from a BCC-only request.

const releasedFile = "released.json"
const maxReleasedJobs = 96

// ReleasedV1 is the durable tombstone of a released reservation.
type ReleasedV1 struct {
	NodeID      string   `json:"node_id"`
	Port        uint16   `json:"port"`
	TunnelID    string   `json:"tunnel_id"`
	InstanceID  string   `json:"instance_id"`
	Generation  uint64   `json:"generation"`
	OperationID string   `json:"operation_id"`
	LeaseID     string   `json:"lease_id"`
	PlanSHA256  string   `json:"plan_sha256"`
	SpentJobs   []string `json:"spent_jobs"`
}

// RetirementEvidenceV1 is what the executor side observed. It is data: the
// guard still re-derives the lifecycle authority and the socket evidence.
type RetirementEvidenceV1 struct {
	Port           uint16
	Owner          ingressmanifest.Proof
	JournalRetired bool
	RuleAbsent     bool
}

var ErrReleaseRefused = errors.New("ingress reservation release refused")

// releaseCollect is the socket inventory used by ReleaseVerified (a seam for
// in-package tests only; production leaves it nil).
var releaseCollect func(context.Context, string, uint16) (ingressinventory.CoverageEvidence, error)

func loadReleased(root *os.Root) (map[string]ReleasedV1, error) {
	out := make(map[string]ReleasedV1)
	f, err := root.OpenFile(releasedFile, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, ErrUnverified
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || int(st.Uid) != os.Geteuid() || st.Nlink != 1 || info.Size() < 2 || info.Size() > reservationMaxBytes {
		return nil, ErrUnverified
	}
	raw, err := io.ReadAll(io.LimitReader(f, reservationMaxBytes+1))
	if err != nil || len(raw) > reservationMaxBytes || json.Unmarshal(raw, &out) != nil || out == nil {
		return nil, ErrUnverified
	}
	canonical, err := json.Marshal(out)
	if err != nil || !bytes.Equal(canonical, raw) {
		return nil, ErrUnverified
	}
	for key, r := range out {
		if key != r.NodeID+"\x00tcp\x00"+fmt.Sprint(r.Port) || r.Generation == 0 || len(r.SpentJobs) > maxReleasedJobs ||
			!idPattern.MatchString(r.OperationID) || !idPattern.MatchString(r.LeaseID) || !digestPattern.MatchString(r.PlanSHA256) {
			return nil, ErrUnverified
		}
		for _, j := range r.SpentJobs {
			if !idPattern.MatchString(j) {
				return nil, ErrUnverified
			}
		}
	}
	return out, nil
}

func writeReleased(d *os.File, root *os.Root, m map[string]ReleasedV1) error {
	raw, err := json.Marshal(m)
	if err != nil || len(raw) > reservationMaxBytes {
		return ErrUnverified
	}
	f, err := root.OpenFile(".released-write", os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(".released-write")
	if _, err = f.Write(raw); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if err = root.Rename(".released-write", releasedFile); err != nil {
		return err
	}
	return d.Sync()
}

// releasedSpent reports whether jobID was spent by any released reservation.
func releasedSpent(rel map[string]ReleasedV1, jobID string) bool {
	for _, r := range rel {
		for _, j := range r.SpentJobs {
			if j == jobID {
				return true
			}
		}
	}
	return false
}

// admitAfterRelease applies the monotonic rule to a NEW binding for a key
// that was released before: strictly greater generation and fresh identities.
func admitAfterRelease(r ReleasedV1, b BindingV1) error {
	if b.Spec.Generation <= r.Generation {
		return fmt.Errorf("%w: generation %d is not greater than the released generation %d", ErrReserved, b.Spec.Generation, r.Generation)
	}
	if b.OperationID == r.OperationID || b.LeaseID == r.LeaseID || b.PlanSHA256 == r.PlanSHA256 {
		return fmt.Errorf("%w: operation, lease and reviewed plan must be fresh after a release", ErrReserved)
	}
	return nil
}

func socketsClear(ctx context.Context, b BindingV1) error {
	collect := releaseCollect
	if collect == nil {
		collect = ingressinventory.CollectInstalledManagedTCP
	}
	obs, err := collect(ctx, b.NodeID, b.Port)
	if err != nil && !errors.Is(err, ingressinventory.ErrClaimConflict) {
		return err
	}
	if !obs.CoverageComplete || len(obs.UnknownSources) != 0 {
		return ingressinventory.ErrCoverageIncomplete
	}
	for _, c := range obs.Stopped {
		if c.Port == b.Port {
			return fmt.Errorf("stopped service claim on port %d", b.Port)
		}
	}
	uid := uint32(os.Geteuid())
	for _, row := range obs.ActiveTCP {
		switch {
		case row.Port != b.Port:
			return errors.New("kernel row for another port")
		case row.State == "0A":
			return fmt.Errorf("a %s listener (inode %d) holds port %d", row.Family, row.Inode, b.Port)
		case row.Inode != 0 && row.Uid == uid:
			return fmt.Errorf("a %s socket of the agent's uid (inode %d, state %s) still exists on port %d", row.Family, row.Inode, row.State, b.Port)
		}
	}
	return ctx.Err()
}

// ReleaseVerified releases this guard's reservation after a verified
// retirement (see the package comment above). On any doubt it changes nothing.
func (g *PendingGuard) ReleaseVerified(ctx context.Context, ev RetirementEvidenceV1) error {
	if g == nil {
		return ErrUnverified
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	refuse := func(why string, args ...any) error {
		return fmt.Errorf("%w: "+why, append([]any{ErrReleaseRefused}, args...)...)
	}
	entry, err := g.restartEntryLocked(ctx)
	if err != nil {
		return refuse("lifecycle: %v", err)
	}
	if _, _, err := restartDirection(entry); err != nil {
		return refuse("no consumed lease on record: %v", err)
	}
	b := g.binding
	if ev.Port != b.Port || ev.Owner != b.Proof || !ev.JournalRetired || !ev.RuleAbsent {
		return refuse("retirement evidence is incomplete or belongs to another resource")
	}
	if err := socketsClear(ctx, b); err != nil {
		return refuse("socket evidence: %v", err)
	}
	// Re-prove the lock and policy after the (slow) kernel read.
	if _, err := g.restartEntryLocked(ctx); err != nil {
		return refuse("lifecycle changed: %v", err)
	}
	key := reservationKey(b)
	rel, err := loadReleased(g.root)
	if err != nil {
		return err
	}
	if old, ok := rel[key]; ok && old.Generation > b.Spec.Generation {
		return refuse("a newer generation %d was already released", old.Generation)
	}
	spent := map[string]bool{}
	for _, c := range entry.Commands {
		spent[c.JobID] = true
	}
	for _, j := range entry.Superseded {
		spent[j] = true
	}
	for _, j := range rel[key].SpentJobs {
		spent[j] = true
	}
	var jobs []string
	for j := range spent {
		jobs = append(jobs, j)
	}
	sort.Strings(jobs)
	if len(jobs) > maxReleasedJobs {
		return refuse("too many spent jobs to record")
	}
	rel[key] = ReleasedV1{NodeID: b.NodeID, Port: b.Port, TunnelID: b.Spec.TunnelID, InstanceID: b.Spec.InstanceID, Generation: b.Spec.Generation,
		OperationID: b.OperationID, LeaseID: b.LeaseID, PlanSHA256: b.PlanSHA256, SpentJobs: jobs}
	// 1. the tombstone (monotonic fence), 2. the lifecycle entry, 3. the claim.
	// A crash after 1 or 2 is completed by completeStaleRelease on the next
	// reservation attempt, from the tombstone alone.
	if err := writeReleased(g.fd, g.root, rel); err != nil {
		return err
	}
	entries, err := readLifecycle(g.root)
	if err != nil {
		return err
	}
	delete(entries, key)
	if err := writeLifecycle(g.fd, g.root, entries); err != nil {
		return err
	}
	claims, err := loadPending(g.root)
	if err != nil {
		return err
	}
	delete(claims, key)
	return savePending(g.fd, g.root, claims)
}

// completeStaleRelease finishes only the metadata tail of a committed release,
// under the store's OS lock. The protected tombstone was persisted AFTER the
// verified retirement; it does not authorize any new listener/firewall effect.
// Match every recorded resource identity, and never discard lifecycle work that
// the tombstone does not account for. Persist lifecycle removal before allowing
// the caller to remove the pending claim, preserving the original write order.
func completeStaleRelease(d *os.File, root *os.Root, key string, claim BindingV1, r ReleasedV1) (bool, error) {
	if key != reservationKey(claim) || r.NodeID != claim.NodeID || r.Port != claim.Port ||
		r.TunnelID != claim.Spec.TunnelID || r.InstanceID != claim.Spec.InstanceID ||
		r.Generation != claim.Spec.Generation || r.OperationID != claim.OperationID || r.LeaseID != claim.LeaseID || r.PlanSHA256 != claim.PlanSHA256 {
		return false, nil
	}
	entries, err := readLifecycle(root)
	if err != nil {
		return false, err
	}
	if entry, hasEntry := entries[key]; hasEntry {
		if entry.Binding != claim {
			return false, nil
		}
		spent := make(map[string]bool, len(r.SpentJobs))
		for _, job := range r.SpentJobs {
			spent[job] = true
		}
		for _, cmd := range entry.Commands {
			if !spent[cmd.JobID] {
				return false, nil
			}
		}
		for _, job := range entry.Superseded {
			if !spent[job] {
				return false, nil
			}
		}
		delete(entries, key)
		if err := writeLifecycle(d, root, entries); err != nil {
			return false, err
		}
	}
	return true, nil
}
