package bcc

// Update Center (v0.4.0 P0).
//
// BCC never applies an update by itself. The Update Center is a verified,
// per-node view of "installed vs latest signed release" plus a hash-bound plan
// that an administrator may then hand to an EXISTING executor: the signed
// `update_baft` agent job (upgrade) or the verified installer (rollback and
// everything the agent job does not replace). Trust comes only from the release
// root pinned at BCC startup: a release is usable only when its manifest is
// signed by a release key certified by that root, the key is not revoked in the
// newest root-signed revocation list BCC has seen, and the version is not below
// the floor unless the operator explicitly imports it as rollback-only.
//
// BCC checks the signature chain and the manifest metadata (exactly the checks
// release.VerifyDir performs before it touches artifact bytes). BCC never holds
// the binaries; the node re-verifies every byte with release.VerifyDir.

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/zarkmakerburg/baft/internal/release"
)

const (
	updateBundleMaxBytes  = 256 << 10
	updateRequestMaxBytes = 16 << 10
	updateMaxReleases     = 64
	updateMaxArtifacts    = 32
	updateMaxSumsBytes    = 16 << 10
	updateReportMaxAge    = 15 * time.Minute
	updateBundleSchema    = 1
)

var (
	updateVersionRe  = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)
	updateCommitRe   = regexp.MustCompile(`^[0-9a-f]{40}$`)
	updateArtifactRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*-linux-(amd64|arm64)$`)
	updateNodeIDRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
)

var (
	errUpdateNoRoot   = errors.New("release root is not pinned on this BCC (start baft-bcc with --update-root-pub)")
	errUpdateRevoked  = errors.New("release signing key is revoked by the root-signed revocation list")
	errUpdateOlder    = errors.New("release is older than the current floor; downgrades are refused unless explicitly imported as rollback-only")
	errUpdateConflict = errors.New("a different release is already imported under this version (re-tagged release refused)")
	errUpdateStale    = errors.New("revocation list is older than one already accepted")
	errUpdateFull     = errors.New("imported release store is full")
)

// UpdateBundle is the operator-imported, size-capped JSON document made from
// the files of an offline bundle (scripts/release/update_center_bundle.sh).
// Manifest and Cert are both set (a release) or both absent (revocation list
// only). Nothing in it is trusted before it verifies against the pinned root.
type UpdateBundle struct {
	SchemaVersion int               `json:"schema_version"`
	Manifest      *release.Envelope `json:"manifest,omitempty"`
	Cert          *release.Envelope `json:"cert,omitempty"`
	Revocations   release.Envelope  `json:"revocations"`
	SHA256SUMS    string            `json:"sha256sums,omitempty"`
}

// UpdateRelease is one imported, signature-verified release. The signed
// envelopes are kept so every read re-verifies against the current root pin.
type UpdateRelease struct {
	Digest       string             `json:"digest"` // sha256 of the signed manifest payload
	Version      string             `json:"version"`
	Commit       string             `json:"commit"`
	CreatedAt    time.Time          `json:"created_at"`
	SigningKeyID string             `json:"signing_key_id"`
	Artifacts    []release.Artifact `json:"artifacts"`
	ReceivedAt   time.Time          `json:"received_at"`
	RollbackOnly bool               `json:"rollback_only,omitempty"`
	Manifest     release.Envelope   `json:"manifest"`
	Cert         release.Envelope   `json:"cert"`
}

// UpdateRevocationState is the newest root-signed revocation list BCC accepted.
// It only ever moves to a higher sequence (also across backup restore).
type UpdateRevocationState struct {
	Sequence      uint64           `json:"sequence"`
	IssuedAt      time.Time        `json:"issued_at"`
	ExpiresAt     time.Time        `json:"expires_at"`
	RevokedKeyIDs []string         `json:"revoked_key_ids"`
	ReceivedAt    time.Time        `json:"received_at"`
	Envelope      release.Envelope `json:"envelope"`
}

func updateChannel(version string) string {
	if strings.Contains(version, "-") {
		return "prerelease"
	}
	return "stable"
}

func parseUpdateBundle(r io.Reader) (UpdateBundle, error) {
	var b UpdateBundle
	body, err := io.ReadAll(io.LimitReader(r, updateBundleMaxBytes+1))
	if err != nil {
		return b, errors.New("unreadable request body")
	}
	if len(body) > updateBundleMaxBytes {
		return b, errTooLarge
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		return b, errors.New("invalid bundle JSON")
	}
	if dec.More() {
		return b, errors.New("trailing data after bundle JSON")
	}
	if b.SchemaVersion != updateBundleSchema {
		return b, fmt.Errorf("unsupported bundle schema %d", b.SchemaVersion)
	}
	if (b.Manifest == nil) != (b.Cert == nil) {
		return b, errors.New("manifest and cert must be given together")
	}
	if len(b.SHA256SUMS) > updateMaxSumsBytes {
		return b, errTooLarge
	}
	return b, nil
}

var errTooLarge = errors.New("request too large")

func strictJSON(payload []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("trailing data after JSON document")
	}
	return nil
}

// openUpdateManifest verifies the signature chain root -> release key ->
// manifest and the manifest's own metadata. It is time-independent so it can be
// re-run on every read; revocation and expiry are judged separately.
func openUpdateManifest(manifestEnv, certEnv release.Envelope, root ed25519.PublicKey) (release.Manifest, release.KeyCert, string, error) {
	var m release.Manifest
	if len(root) != ed25519.PublicKeySize {
		return m, release.KeyCert{}, "", errUpdateNoRoot
	}
	cert, relPub, err := release.OpenCert(certEnv, root)
	if err != nil {
		return m, cert, "", err
	}
	payload, err := manifestEnv.Open(release.PayloadTypeManifest, relPub)
	if err != nil {
		return m, cert, "", fmt.Errorf("manifest: %w", err)
	}
	if err := strictJSON(payload, &m); err != nil {
		return m, cert, "", fmt.Errorf("manifest: %w", err)
	}
	if m.SchemaVersion != 1 || m.Product != release.Product {
		return m, cert, "", fmt.Errorf("manifest: unsupported schema %d / product %q", m.SchemaVersion, m.Product)
	}
	if !updateVersionRe.MatchString(m.Version) || len(m.Version) > 64 || !updateCommitRe.MatchString(m.Commit) {
		return m, cert, "", errors.New("manifest: malformed version or commit")
	}
	if _, err := release.CompareVersions(m.Version, m.Version); err != nil {
		return m, cert, "", fmt.Errorf("manifest: %w", err)
	}
	if m.SigningKeyID != cert.KeyID {
		return m, cert, "", errors.New("manifest: signing_key_id does not match certificate")
	}
	if m.CreatedAt.Before(cert.NotBefore) || m.CreatedAt.After(cert.NotAfter) {
		return m, cert, "", errors.New("manifest: created_at is outside the release key's validity window")
	}
	if len(m.Artifacts) == 0 || len(m.Artifacts) > updateMaxArtifacts {
		return m, cert, "", errors.New("manifest: unexpected artifact count")
	}
	seen := map[string]bool{}
	for _, a := range m.Artifacts {
		if !updateArtifactRe.MatchString(a.Name) || seen[a.Name] || a.Size <= 0 || !isHexSHA256(a.SHA256) {
			return m, cert, "", fmt.Errorf("manifest: malformed or duplicate artifact %q", a.Name)
		}
		seen[a.Name] = true
	}
	if !isHexSHA256(m.SHA256SUMS) {
		return m, cert, "", errors.New("manifest: malformed sha256sums_sha256")
	}
	sum := sha256.Sum256(payload)
	return m, cert, hex.EncodeToString(sum[:]), nil
}

// checkUpdateSums mirrors VerifyDir: SHA256SUMS must match the signed hash and
// list exactly the signed artifacts.
func checkUpdateSums(m release.Manifest, sums string) error {
	h := sha256.Sum256([]byte(sums))
	if hex.EncodeToString(h[:]) != m.SHA256SUMS {
		return errors.New("SHA256SUMS does not match the signed manifest")
	}
	listed := map[string]string{}
	for _, line := range strings.Split(strings.TrimRight(sums, "\n"), "\n") {
		hash, name, ok := strings.Cut(line, "  ")
		if !ok || len(hash) != 64 || name == "" {
			return errors.New("SHA256SUMS is malformed")
		}
		if _, dup := listed[name]; dup {
			return errors.New("SHA256SUMS lists a file twice")
		}
		listed[name] = hash
	}
	if len(listed) != len(m.Artifacts) {
		return errors.New("SHA256SUMS and manifest list different artifacts")
	}
	for _, a := range m.Artifacts {
		if listed[a.Name] != a.SHA256 {
			return fmt.Errorf("%s: SHA256SUMS and manifest disagree", a.Name)
		}
	}
	return nil
}

// verifyUpdateBundle verifies everything an import must prove. A nil release
// means a revocation-list-only bundle.
func verifyUpdateBundle(b UpdateBundle, root ed25519.PublicKey, now time.Time) (*UpdateRelease, UpdateRevocationState, error) {
	var zero UpdateRevocationState
	if len(root) != ed25519.PublicKeySize {
		return nil, zero, errUpdateNoRoot
	}
	rev, err := release.OpenRevocations(b.Revocations, root)
	if err != nil {
		return nil, zero, err
	}
	if !now.Before(rev.ExpiresAt) {
		return nil, zero, fmt.Errorf("revocation list: expired at %s; the root holder must re-sign it", rev.ExpiresAt.Format(time.RFC3339))
	}
	state := UpdateRevocationState{
		Sequence: rev.Sequence, IssuedAt: rev.IssuedAt, ExpiresAt: rev.ExpiresAt,
		RevokedKeyIDs: append([]string(nil), rev.RevokedKeyIDs...), ReceivedAt: now.UTC(), Envelope: b.Revocations,
	}
	if b.Manifest == nil {
		return nil, state, nil
	}
	m, cert, digest, err := openUpdateManifest(*b.Manifest, *b.Cert, root)
	if err != nil {
		return nil, zero, err
	}
	for _, id := range rev.RevokedKeyIDs {
		if id == cert.KeyID {
			return nil, zero, errUpdateRevoked
		}
	}
	if b.SHA256SUMS == "" {
		return nil, zero, errors.New("sha256sums is required with a release")
	}
	if err := checkUpdateSums(m, b.SHA256SUMS); err != nil {
		return nil, zero, err
	}
	rel := &UpdateRelease{
		Digest: digest, Version: m.Version, Commit: m.Commit, CreatedAt: m.CreatedAt.UTC(), SigningKeyID: m.SigningKeyID,
		Artifacts: append([]release.Artifact(nil), m.Artifacts...), ReceivedAt: now.UTC(),
		Manifest: *b.Manifest, Cert: *b.Cert,
	}
	return rel, state, nil
}

// ---- read model ----

type UpdateReleaseView struct {
	Digest          string    `json:"digest"`
	Version         string    `json:"version"`
	Commit          string    `json:"commit"`
	Channel         string    `json:"channel"`
	CreatedAt       time.Time `json:"created_at"`
	ReceivedAt      time.Time `json:"received_at"`
	SignatureStatus string    `json:"signature_status"` // verified | invalid | unpinned
	Revoked         bool      `json:"revoked"`
	RollbackOnly    bool      `json:"rollback_only"`
}

type UpdateComponentView struct {
	Component string              `json:"component"`
	State     ComponentTrustState `json:"state"`
	Version   string              `json:"version,omitempty"`
}

type UpdateNodeRow struct {
	NodeID           string                `json:"node_id"`
	Alias            string                `json:"alias,omitempty"`
	Role             string                `json:"role"`
	InstalledVersion string                `json:"installed_version,omitempty"`
	InstalledDigest  string                `json:"installed_release,omitempty"`
	LatestVersion    string                `json:"latest_version,omitempty"`
	Status           string                `json:"status"` // current | update-available | downgrade-blocked | unknown | revoked
	NoticeCode       string                `json:"notice_code"`
	Notice           string                `json:"notice"`
	LastReportAge    *int64                `json:"last_report_age_seconds,omitempty"`
	Components       []UpdateComponentView `json:"components"`
}

type UpdateView struct {
	RootPinned         bool                `json:"root_pinned"`
	Floor              string              `json:"floor,omitempty"`
	RevocationSequence uint64              `json:"revocation_sequence"`
	RevocationExpired  bool                `json:"revocation_expired"`
	RevocationExpires  *time.Time          `json:"revocation_expires_at,omitempty"`
	Releases           []UpdateReleaseView `json:"releases"`
	Latest             *UpdateReleaseView  `json:"latest,omitempty"`
	Nodes              []UpdateNodeRow     `json:"nodes"`
	NoticeCount        int                 `json:"notice_count"`
	BCCVersion         string              `json:"bcc_version,omitempty"`
}

var updateNoticeText = map[string]string{
	"update_available":  "A newer verified release is available; review the plan before applying it.",
	"current":           "Running the latest verified release.",
	"revoked_installed": "The installed release was signed with a revoked key; move to a verified release.",
	"ahead_of_catalog":  "Installed version is newer than the latest imported release; no downgrade is offered.",
	"node_revoked":      "Access for this server is revoked.",
	"no_platform":       "Operator-confirmed platform is missing; confirm OS/architecture first.",
	"no_report":         "No component report from this server yet.",
	"stale_report":      "The last component report is too old to trust.",
	"unmatched_digest":  "Installed binary does not match any imported signed release.",
	"no_release":        "No verified release is imported; import a signed bundle.",
	"no_root":           "The release root is not pinned on this BCC.",
}

type updateEval struct {
	view     UpdateView
	releases map[string]UpdateRelease // by digest, verified signature only
	rev      *UpdateRevocationState
	revoked  map[string]bool          // key id
	all      map[string]UpdateRelease // every imported release, verified or not
	nodes    []Node
}

// evalUpdates is the single place that decides signature status, revocation,
// eligibility and per-node status. Everything else formats its result.
func evalUpdates(nodes []Node, receipts map[string]ComponentDigestReceipt, releases map[string]UpdateRelease,
	rev *UpdateRevocationState, root ed25519.PublicKey, floor, bccVersion string, now time.Time) updateEval {
	ev := updateEval{releases: map[string]UpdateRelease{}, rev: rev, revoked: map[string]bool{}, all: releases, nodes: nodes}
	v := &ev.view
	v.RootPinned = len(root) == ed25519.PublicKeySize
	v.Floor = floor
	v.BCCVersion = bccVersion
	v.Releases = []UpdateReleaseView{}
	v.Nodes = []UpdateNodeRow{}
	if rev != nil {
		v.RevocationSequence = rev.Sequence
		exp := rev.ExpiresAt
		v.RevocationExpires = &exp
		v.RevocationExpired = !now.Before(rev.ExpiresAt)
		for _, id := range rev.RevokedKeyIDs {
			ev.revoked[id] = true
		}
	}
	var latest *UpdateRelease
	for _, r := range releases {
		rv := UpdateReleaseView{
			Digest: r.Digest, Version: r.Version, Commit: r.Commit, Channel: updateChannel(r.Version),
			CreatedAt: r.CreatedAt, ReceivedAt: r.ReceivedAt, RollbackOnly: r.RollbackOnly, SignatureStatus: "unpinned",
		}
		if v.RootPinned {
			rv.SignatureStatus = "invalid"
			if m, _, d, err := openUpdateManifest(r.Manifest, r.Cert, root); err == nil && d == r.Digest && m.Version == r.Version && m.Commit == r.Commit {
				rv.SignatureStatus = "verified"
				ev.releases[r.Digest] = r
			}
		}
		rv.Revoked = ev.revoked[r.SigningKeyID]
		v.Releases = append(v.Releases, rv)
		if rv.SignatureStatus == "verified" && !rv.Revoked && !r.RollbackOnly {
			if latest == nil {
				c := r
				latest = &c
			} else if c, err := release.CompareVersions(r.Version, latest.Version); err == nil && c > 0 {
				cp := r
				latest = &cp
			}
		}
	}
	sort.Slice(v.Releases, func(i, j int) bool {
		c, err := release.CompareVersions(v.Releases[i].Version, v.Releases[j].Version)
		if err != nil || c == 0 {
			return v.Releases[i].Digest < v.Releases[j].Digest
		}
		return c > 0
	})
	if latest != nil {
		for i := range v.Releases {
			if v.Releases[i].Digest == latest.Digest {
				lv := v.Releases[i]
				v.Latest = &lv
			}
		}
	}
	sorted := append([]Node(nil), nodes...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	for _, n := range sorted {
		row := ev.nodeRow(n, receipts[n.ID], latest, now)
		if row.Status == "update-available" || row.Status == "revoked" {
			v.NoticeCount++
		}
		v.Nodes = append(v.Nodes, row)
	}
	return ev
}

// matchComponent returns the imported release whose signed artifact digest for
// the node's confirmed platform equals the digest the node reported.
func (ev *updateEval) matchComponent(component string, n Node, receipt ComponentDigestReceipt) (UpdateRelease, bool) {
	var best UpdateRelease
	found := false
	artifact, ok := TrustedNodeArtifactName(component, n.Platform)
	if !ok {
		return best, false
	}
	for _, r := range ev.releases {
		verified := release.Verified{Manifest: release.Manifest{Version: r.Version, Commit: r.Commit, Artifacts: r.Artifacts}}
		if CompareVerifiedComponentDigest(receipt.Digests[component], verified, artifact) != ComponentVerified {
			continue
		}
		if !found {
			best, found = r, true
		} else if c, err := release.CompareVersions(r.Version, best.Version); err == nil && c > 0 {
			best = r
		}
	}
	return best, found
}

func (ev *updateEval) nodeRow(n Node, receipt ComponentDigestReceipt, latest *UpdateRelease, now time.Time) UpdateNodeRow {
	row := UpdateNodeRow{NodeID: n.ID, Alias: n.Alias, Role: n.Role, Components: []UpdateComponentView{}}
	set := func(status, code string) UpdateNodeRow {
		row.Status, row.NoticeCode = status, code
		row.Notice = updateNoticeText[code]
		return row
	}
	reported := receipt.NodeID == n.ID && !receipt.AcceptedAt.IsZero()
	fresh := reported && !receipt.AcceptedAt.After(now) && now.Sub(receipt.AcceptedAt) <= updateReportMaxAge &&
		!n.Platform.ConfirmedAt.IsZero() && !receipt.AcceptedAt.Before(n.Platform.ConfirmedAt)
	if reported {
		age := int64(now.Sub(receipt.AcceptedAt) / time.Second)
		if age < 0 {
			age = 0
		}
		row.LastReportAge = &age
	}
	if latest != nil {
		row.LatestVersion = latest.Version
	}
	for _, comp := range []string{"transport", "agent", "bcc"} {
		cv := UpdateComponentView{Component: comp, State: ComponentUnknown}
		if fresh && !n.Revoked {
			if r, ok := ev.matchComponent(comp, n, receipt); ok {
				cv.State, cv.Version = ComponentVerified, r.Version
			} else if _, ok := TrustedNodeArtifactName(comp, n.Platform); ok && isHexSHA256(receipt.Digests[comp]) && len(ev.releases) > 0 {
				cv.State = ComponentMismatch
			}
		}
		row.Components = append(row.Components, cv)
	}
	switch {
	case n.Revoked:
		return set("unknown", "node_revoked")
	case !ev.view.RootPinned:
		return set("unknown", "no_root")
	case !(n.Platform.Confirmed && n.Platform.Source == "operator-verified"):
		return set("unknown", "no_platform")
	case !reported || !isHexSHA256(receipt.Digests["transport"]):
		return set("unknown", "no_report")
	case !fresh:
		return set("unknown", "stale_report")
	case len(ev.releases) == 0:
		return set("unknown", "no_release")
	}
	inst, ok := ev.matchComponent("transport", n, receipt)
	if !ok {
		return set("unknown", "unmatched_digest")
	}
	row.InstalledVersion, row.InstalledDigest = inst.Version, inst.Digest
	if ev.revoked[inst.SigningKeyID] {
		return set("revoked", "revoked_installed")
	}
	if latest == nil {
		return set("unknown", "no_release")
	}
	c, err := release.CompareVersions(inst.Version, latest.Version)
	switch {
	case err != nil:
		return set("unknown", "unmatched_digest")
	case c < 0:
		return set("update-available", "update_available")
	case c == 0:
		return set("current", "current")
	}
	return set("downgrade-blocked", "ahead_of_catalog")
}

// updateFloor is the lowest version an import may carry without the explicit
// rollback-only confirmation: the newer of the BCC's own build and the version
// the signed component catalog last accepted.
func (s *Server) updateFloor() string {
	floor := ""
	consider := func(v string) {
		if !updateVersionRe.MatchString(v) {
			return
		}
		if floor == "" {
			floor = v
		} else if c, err := release.CompareVersions(v, floor); err == nil && c > 0 {
			floor = v
		}
	}
	consider(s.version)
	if s.verifiedCatalog != nil {
		if st, err := release.ReadState(s.verifiedCatalog.statePath); err == nil {
			consider(st.Version)
		}
	}
	return floor
}

// updateRoot is the pinned release root: the dedicated update pin, else the
// signed component catalog's. Nothing a caller sends can change it.
func (s *Server) updateRoot() ed25519.PublicKey {
	if len(s.updateRootKey) == ed25519.PublicKeySize {
		return s.updateRootKey
	}
	if s.verifiedCatalog != nil {
		return s.verifiedCatalog.root
	}
	return nil
}

// ConfigureUpdateCenter pins the release root the Update Center verifies
// imports against. Without it (and without a signed catalog) imports are refused.
func (s *Server) ConfigureUpdateCenter(root ed25519.PublicKey) error {
	if len(root) != ed25519.PublicKeySize {
		return errors.New("update root must be an Ed25519 public key")
	}
	s.updateRootKey = append(ed25519.PublicKey(nil), root...)
	return nil
}

func (s *Server) updateEval() updateEval {
	nodes, receipts := s.store.ComponentVersionSnapshot()
	rels, rev := s.store.UpdateCenterSnapshot()
	return evalUpdates(nodes, receipts, rels, rev, s.updateRoot(), s.updateFloor(), s.version, s.now())
}
