package bcc

import (
	"github.com/zarkmakerburg/baft/internal/release"
)

// UpdateCenterSnapshot returns copies of the imported releases and the newest
// accepted revocation list, from one consistent state lock.
func (s *Store) UpdateCenterSnapshot() (map[string]UpdateRelease, *UpdateRevocationState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rels := make(map[string]UpdateRelease, len(s.st.UpdateReleases))
	for k, v := range s.st.UpdateReleases {
		rels[k] = cloneUpdateRelease(v)
	}
	var rev *UpdateRevocationState
	if s.st.UpdateRevocation != nil {
		c := *s.st.UpdateRevocation
		c.RevokedKeyIDs = append([]string(nil), c.RevokedKeyIDs...)
		rev = &c
	}
	return rels, rev
}

func cloneUpdateRelease(r UpdateRelease) UpdateRelease {
	r.Artifacts = append([]release.Artifact(nil), r.Artifacts...)
	return r
}

// UpdateImportResult tells the caller what an import changed.
type UpdateImportResult struct {
	Release         *UpdateRelease
	Created         bool
	RevocationAdded bool
}

// ImportUpdateRelease durably records a verified release and/or revocation
// list together with its audit intent (one SQLite transaction). rel may be nil
// for a revocation-only bundle. floor "" means no floor. A release below the
// floor is refused unless allowRollbackOnly, which stores it as rollback-only.
func (s *Store) ImportUpdateRelease(rel *UpdateRelease, rev UpdateRevocationState, floor string, allowRollbackOnly bool, audit AuditEntry) (UpdateImportResult, error) {
	var out UpdateImportResult
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.st.UpdateRevocation
	if cur != nil && rev.Sequence < cur.Sequence {
		return out, errUpdateStale
	}
	if rel != nil {
		if existing, ok := s.st.UpdateReleases[rel.Digest]; ok {
			cp := cloneUpdateRelease(existing)
			out.Release = &cp
		} else {
			for _, other := range s.st.UpdateReleases {
				if other.Version == rel.Version {
					return out, errUpdateConflict
				}
			}
			if len(s.st.UpdateReleases) >= updateMaxReleases {
				return out, errUpdateFull
			}
			if floor != "" {
				c, err := release.CompareVersions(rel.Version, floor)
				if err != nil {
					return out, err
				}
				if c < 0 {
					if !allowRollbackOnly {
						return out, errUpdateOlder
					}
					rel.RollbackOnly = true
				}
			}
			out.Created = true
		}
	}
	out.RevocationAdded = cur == nil || rev.Sequence > cur.Sequence
	if !out.Created && !out.RevocationAdded {
		return out, nil // idempotent re-import: nothing to write
	}
	before, err := cloneState(s.st)
	if err != nil {
		return out, err
	}
	if out.Created {
		if s.st.UpdateReleases == nil {
			s.st.UpdateReleases = map[string]UpdateRelease{}
		}
		s.st.UpdateReleases[rel.Digest] = cloneUpdateRelease(*rel)
		cp := cloneUpdateRelease(*rel)
		out.Release = &cp
	}
	if out.RevocationAdded {
		r := rev
		r.RevokedKeyIDs = append([]string(nil), r.RevokedKeyIDs...)
		s.st.UpdateRevocation = &r
	}
	audit.Action = "updates.import"
	audit.Outcome = "success"
	if out.Release != nil {
		audit.Target = out.Release.Version
		audit.Details = mergeAuditDetails(audit.Details, map[string]any{
			"version": out.Release.Version, "commit": out.Release.Commit, "digest": out.Release.Digest,
			"rollback_only": out.Release.RollbackOnly, "created": out.Created,
		})
	} else {
		audit.Target = "revocations"
	}
	audit.Details = mergeAuditDetails(audit.Details, map[string]any{"revocation_sequence": rev.Sequence, "revocation_updated": out.RevocationAdded})
	if audit.Actor == "" {
		audit.Actor = "admin"
	}
	if _, err := s.enqueueSecurityAuditLocked(audit); err != nil {
		s.st = before
		return UpdateImportResult{}, err
	}
	if err := s.saveLocked(); err != nil {
		s.st = before
		return UpdateImportResult{}, err
	}
	return out, nil
}

// updateDeployInFlight reports a deploy job for the node that has not finished.
func (s *Store) updateDeployInFlight(nodeID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, j := range s.st.Jobs {
		if j.Type == JobDeployBAFT && j.NodeID == nodeID && j.Status != "succeeded" && j.Status != "failed" {
			return true
		}
	}
	return false
}

// mergeUpdateCenter is the restore anti-rollback rule for the Update Center:
// restoring an older backup must never forget an imported release or resurrect
// an older (less revoking) revocation list. Releases are unioned; the higher
// revocation sequence wins; a rollback-only mark set by the live state sticks.
func mergeUpdateCenter(restored *state, current state) {
	if len(current.UpdateReleases) > 0 && restored.UpdateReleases == nil {
		restored.UpdateReleases = map[string]UpdateRelease{}
	}
	for d, r := range current.UpdateReleases {
		if old, ok := restored.UpdateReleases[d]; ok {
			r.RollbackOnly = r.RollbackOnly || old.RollbackOnly
		}
		restored.UpdateReleases[d] = cloneUpdateRelease(r)
	}
	if current.UpdateRevocation != nil && (restored.UpdateRevocation == nil || current.UpdateRevocation.Sequence > restored.UpdateRevocation.Sequence) {
		c := *current.UpdateRevocation
		c.RevokedKeyIDs = append([]string(nil), c.RevokedKeyIDs...)
		restored.UpdateRevocation = &c
	}
}
