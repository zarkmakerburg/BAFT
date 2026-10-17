// Package selfupdate looks for a newer signed BAFT release without changing
// the host.
//
// It downloads the release metadata and the signed installer artifact into a
// scratch directory and verifies them with internal/release against the
// pinned root key, the root-signed revocation list and the host's trust
// state, so an unsigned, revoked, replayed, re-tagged or older release is
// refused. It never writes the trust state and never runs anything it
// downloaded: applying a release stays with the installer, which verifies the
// binaries again before it touches the host.
//
// A check that cannot reach or verify a release reports Unknown with the
// reason. It never reports "current" on a failure.
package selfupdate

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/zarkmakerburg/baft/internal/release"
)

// DefaultRepo is where install.sh looks for releases unless told otherwise.
const DefaultRepo = "zarkmakerburg/baft"

// InstallerArtifact is the signed installer's artifact name for an
// architecture. It is byte-identical to install.sh at the release commit.
func InstallerArtifact(arch string) string { return "baft-install-linux-" + arch }

// Size caps for what is downloaded, so a hostile or broken server cannot fill
// the disk or memory. The installer is ~150 KiB today.
const (
	maxMetaBytes      = 1 << 20
	maxInstallerBytes = 8 << 20
	requestTimeout    = 30 * time.Second
)

var (
	repoRe    = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	versionRe = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)
)

// Status is the outcome of a check.
type Status string

const (
	Current         Status = "current"
	UpdateAvailable Status = "update_available"
	// Unknown means nothing is claimed about whether an update exists: the
	// release could not be fetched or verified, or the installed version is
	// not known.
	Unknown Status = "unknown"
)

// Report is the result of one check. Latest, Commit and the signing fields are
// set only for a release that passed verification.
type Report struct {
	Status             Status    `json:"status"`
	Installed          string    `json:"installed,omitempty"`
	Latest             string    `json:"latest,omitempty"`
	Commit             string    `json:"commit,omitempty"`
	SigningKeyID       string    `json:"signing_key_id,omitempty"`
	ReleasedAt         time.Time `json:"released_at,omitzero"`
	RevocationSequence uint64    `json:"revocation_sequence,omitempty"`
	CheckedAt          time.Time `json:"checked_at"`
	Reason             string    `json:"reason,omitempty"`
	// Installer is the verified installer inside the scratch directory; it is
	// set only when verification passed.
	Installer string `json:"-"`
}

// Source says where releases come from. The zero value is the project's
// GitHub releases, like install.sh.
type Source struct {
	// Repo is "owner/name"; empty means DefaultRepo.
	Repo string
	// ReleaseURL, when set, is the directory URL holding the release assets
	// and replaces the GitHub location (install.sh's BAFT_RELEASE_URL).
	ReleaseURL string
	// RevocationsURL overrides the revocation list location.
	RevocationsURL string
	// Client is the HTTP client; nil means one with a 30 s timeout.
	Client *http.Client
}

// Config is everything a check needs besides the version to look for.
type Config struct {
	// Root is the pinned release root key (release/keys.Root()).
	Root ed25519.PublicKey
	Source
	// StatePath is the host's release/trust state. It is only read.
	StatePath string
	// Arch is "amd64" or "arm64".
	Arch string
	// Now is the verification time; nil means time.Now.
	Now func() time.Time
}

// Check downloads the release tagged version (empty means the latest) into
// dir, which must be an existing directory the caller owns and removes, and
// verifies it. dir receives a release/ subdirectory and revocations.json.
func Check(ctx context.Context, cfg Config, version, dir string) Report {
	now := time.Now
	if cfg.Now != nil {
		now = cfg.Now
	}
	rep := Report{Status: Unknown, CheckedAt: now().UTC()}
	fail := func(format string, a ...any) Report {
		rep.Reason = fmt.Sprintf(format, a...)
		return rep
	}

	if cfg.Arch != "amd64" && cfg.Arch != "arm64" {
		return fail("unsupported architecture %q", cfg.Arch)
	}
	if len(cfg.Root) != ed25519.PublicKeySize {
		return fail("no pinned release root key")
	}
	if version != "" && !versionRe.MatchString(version) {
		return fail("%q is not a release version like v1.2.3", version)
	}
	state, err := release.ReadState(cfg.StatePath)
	if err != nil {
		return fail("the host's release state cannot be read: %v", err)
	}
	rep.Installed = state.Version

	base, revURL, err := cfg.Source.urls(version)
	if err != nil {
		return fail("%v", err)
	}
	relDir := filepath.Join(dir, "release")
	if err := os.Mkdir(relDir, 0o700); err != nil {
		return fail("cannot create a scratch directory: %v", err)
	}
	installer := InstallerArtifact(cfg.Arch)
	for _, f := range []struct {
		name  string
		limit int64
	}{
		{release.ManifestFile, maxMetaBytes},
		{release.CertFile, maxMetaBytes},
		{release.SumsFile, maxMetaBytes},
		{installer, maxInstallerBytes},
	} {
		if err := cfg.Source.download(ctx, base+"/"+f.name, filepath.Join(relDir, f.name), f.limit); err != nil {
			return fail("cannot download the release: %v", err)
		}
	}
	revPath := filepath.Join(dir, "revocations.json")
	if err := cfg.Source.download(ctx, revURL, revPath, maxMetaBytes); err != nil {
		return fail("cannot download the revocation list: %v", err)
	}
	revEnv, err := release.ReadEnvelope(revPath)
	if err != nil {
		return fail("revocation list: %v", err)
	}

	v, err := release.VerifyDir(release.VerifyInput{
		Dir:         relDir,
		Root:        cfg.Root,
		Revocations: &revEnv,
		Now:         now(),
		State:       &state,
		Only:        []string{installer},
	})
	if err != nil {
		return fail("the release did not verify and was not trusted: %v", err)
	}
	if version != "" && v.Version != version {
		return fail("the release published as %s declares itself %s; refused", version, v.Version)
	}
	rep.Latest, rep.Commit, rep.SigningKeyID = v.Version, v.Commit, v.SigningKeyID
	rep.ReleasedAt, rep.RevocationSequence = v.CreatedAt.UTC(), v.Revocations.Sequence
	rep.Installer = filepath.Join(relDir, installer)

	if state.Version == "" {
		rep.Installer = ""
		return fail("this host has no release state (a source install?), so its version is unknown; the newest signed release is %s", v.Version)
	}
	cmp, err := release.CompareVersions(v.Version, state.Version)
	switch {
	case err != nil:
		rep.Installer = ""
		return fail("cannot compare %s with %s: %v", v.Version, state.Version, err)
	case cmp < 0:
		rep.Installer = ""
		return fail("%s is older than the installed %s; a downgrade is never offered", v.Version, state.Version)
	case cmp == 0 && state.Commit != "" && v.Commit != state.Commit:
		rep.Installer = ""
		return fail("%s is already installed from commit %s but the release says %s; refused", v.Version, state.Commit, v.Commit)
	case cmp == 0:
		rep.Status = Current
	default:
		rep.Status = UpdateAvailable
	}
	return rep
}

// urls returns the directory holding the release assets and the revocation
// list location.
func (s Source) urls(version string) (base, revocations string, err error) {
	repo := s.Repo
	if repo == "" {
		repo = DefaultRepo
	}
	if !repoRe.MatchString(repo) {
		return "", "", fmt.Errorf("%q is not a repository like owner/name", repo)
	}
	base = s.ReleaseURL
	if base == "" {
		if version == "" {
			base = "https://github.com/" + repo + "/releases/latest/download"
		} else {
			base = "https://github.com/" + repo + "/releases/download/" + version
		}
	}
	revocations = s.RevocationsURL
	if revocations == "" {
		revocations = "https://raw.githubusercontent.com/" + repo + "/main/release/keys/revocations.json"
	}
	for _, u := range []string{base, revocations} {
		if err := httpsURL(u); err != nil {
			return "", "", err
		}
	}
	return trimSlash(base), revocations, nil
}

func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

func httpsURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%q is not a usable URL", raw)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("%s: only https is accepted", u.Redacted())
	}
	if u.Host == "" {
		return fmt.Errorf("%q is not a usable URL", raw)
	}
	return nil
}

func httpsOnlyRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errors.New("too many redirects")
	}
	if req.URL.Scheme != "https" {
		return fmt.Errorf("redirect to %s refused: only https is accepted", req.URL.Redacted())
	}
	return nil
}

// download fetches rawURL into path (mode 0600), refusing any body larger than
// limit and anything but a 200 response.
func (s Source) download(ctx context.Context, rawURL, path string, limit int64) error {
	if err := httpsURL(rawURL); err != nil {
		return err
	}
	c := &http.Client{Timeout: requestTimeout}
	if s.Client != nil {
		cc := *s.Client
		c = &cc
	}
	if c.CheckRedirect == nil {
		c.CheckRedirect = httpsOnlyRedirect
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d", filepath.Base(path), resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	if int64(len(body)) > limit {
		return fmt.Errorf("%s: larger than %d bytes", filepath.Base(path), limit)
	}
	return os.WriteFile(path, body, 0o600)
}
