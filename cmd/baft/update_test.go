package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/release"
	"github.com/zarkmakerburg/baft/internal/selfupdate"
)

const stubInstaller = "#!/usr/bin/env bash\necho signed installer\n"

var (
	checkedAt       = time.Date(2026, 10, 17, 12, 30, 0, 0, time.UTC)
	reportAvailable = selfupdate.Report{
		Status: selfupdate.UpdateAvailable, Installed: "v0.3.3", Latest: "v0.3.4",
		Commit: "4545a784cb6c043ac1f97f88c810c422144d7823", SigningKeyID: "702c62788d60920da8ea80aab93ca3e1",
		ReleasedAt: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC), RevocationSequence: 1, CheckedAt: checkedAt,
		Artifacts: map[string]string{selfupdate.InstallerArtifact(runtime.GOARCH): hexSum(stubInstaller)},
	}
	reportCurrent = func() selfupdate.Report {
		r := reportAvailable
		r.Status, r.Installed = selfupdate.Current, "v0.3.4"
		return r
	}()
	reportUnknown = selfupdate.Report{
		Status: selfupdate.Unknown, Installed: "v0.3.3", CheckedAt: checkedAt,
		Reason: "cannot download the release: manifest.json: HTTP 404",
	}
)

type updateStub struct {
	rep   selfupdate.Report
	calls []string // statePath|version
	dirs  []string
}

// install makes the host's update check return the stub's report; when the
// report is verified it also leaves the signed installer in the scratch dir.
func (s *updateStub) install(h *fakeHost) {
	h.env.checkUpdate = func(_ context.Context, statePath, version, dir string) selfupdate.Report {
		s.calls = append(s.calls, statePath+"|"+version)
		s.dirs = append(s.dirs, dir)
		rep := s.rep
		if rep.Status != selfupdate.Unknown {
			p := filepath.Join(dir, "baft-install-linux-amd64")
			if err := os.WriteFile(p, []byte(stubInstaller), 0o600); err != nil {
				panic(err)
			}
			rep.Installer = p
		}
		return rep
	}
}

func runUpdateArgs(t *testing.T, h *fakeHost, args ...string) (code int, out, errOut string) {
	t.Helper()
	var o, e bytes.Buffer
	code = runUpdate(context.Background(), args, strings.NewReader(""), &o, &e, h.env)
	return code, o.String(), e.String()
}

func TestUpdateWithoutASubcommandRunsNothingAndSaysSo(t *testing.T) {
	h := newFakeHost(t)
	for _, args := range [][]string{nil, {"banana"}, {"--check"}, {"check", "extra"}, {"check", "--nope"}, {"apply", "--nope"}, {"apply", "--json"}} {
		code, out, errOut := runUpdateArgs(t, h, args...)
		if code != 2 || out != "" || !strings.Contains(errOut, "usage: baft update check") {
			t.Errorf("%v: code=%d out=%q err=%q", args, code, out, errOut)
		}
	}
	if _, _, e := runUpdateArgs(t, h); !strings.Contains(e, "baft update apply [--plan] [--yes]") {
		t.Error("the usage does not mention apply")
	}
	if code, out, e := runUpdateArgs(t, h, "apply", "--only", "database"); code != 2 || out != "" || !strings.Contains(e, `"database" is not transport, agent or bcc`) {
		t.Errorf("--only database: code=%d out=%q err=%q", code, out, e)
	}
}

func TestUpdateCheckExitCodesFollowTheStatus(t *testing.T) {
	for _, c := range []struct {
		name string
		rep  selfupdate.Report
		code int
		want []string
	}{
		{"update available", reportAvailable, exitUpdateAvailable, []string{"UPDATE AVAILABLE", "installed     v0.3.3", "newest        v0.3.4 (signed; commit 4545a784cb6c;", "signing key   702c62788d60920da8ea80aab93ca3e1, revocation list #1", "baft update apply --plan", "BAFT_VERSION=v0.3.4 bash FILE --plan", "Nothing on this host was changed"}},
		{"current", reportCurrent, 0, []string{"status        CURRENT", "newest verified signed release. Nothing was changed."}},
		{"unknown", reportUnknown, 1, []string{"status        UNKNOWN", "reason        cannot download the release: manifest.json: HTTP 404", "No claim is made about whether an update exists"}},
	} {
		h := newFakeHost(t)
		stub := &updateStub{rep: c.rep}
		stub.install(h)
		code, out, errOut := runUpdateArgs(t, h, "check", "--release-state", "/state/rs.json")
		if code != c.code || errOut != "" {
			t.Errorf("%s: code=%d stderr=%q", c.name, code, errOut)
		}
		for _, w := range c.want {
			if !strings.Contains(out, w) {
				t.Errorf("%s: output lacks %q:\n%s", c.name, w, out)
			}
		}
		if len(stub.calls) != 1 || stub.calls[0] != "/state/rs.json|" {
			t.Errorf("%s: the check was called as %v", c.name, stub.calls)
		}
		if c.rep.Status == selfupdate.Unknown && (strings.Contains(out, "CURRENT") || strings.Contains(out, "UPDATE AVAILABLE") || strings.Contains(out, "up to date")) {
			t.Errorf("an unknown result reads as a verdict:\n%s", out)
		}
		if _, err := os.Stat(stub.dirs[0]); !os.IsNotExist(err) {
			t.Errorf("%s: the scratch directory survived: %v", c.name, err)
		}
	}
}

func TestUpdateCheckPassesTheRequestedVersion(t *testing.T) {
	h := newFakeHost(t)
	stub := &updateStub{rep: reportAvailable}
	stub.install(h)
	runUpdateArgs(t, h, "check", "--version", "v0.3.4")
	if len(stub.calls) != 1 || !strings.HasSuffix(stub.calls[0], "|v0.3.4") || !strings.HasPrefix(stub.calls[0], defaultReleaseState+"|") {
		t.Fatalf("calls = %v", stub.calls)
	}
}

func TestUpdateCheckJSON(t *testing.T) {
	h := newFakeHost(t)
	stub := &updateStub{rep: reportAvailable}
	stub.install(h)
	dest := filepath.Join(t.TempDir(), "installer.sh")
	code, out, _ := runUpdateArgs(t, h, "check", "--json", "--save-installer", dest)
	if code != exitUpdateAvailable {
		t.Fatalf("code = %d", code)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	for k, want := range map[string]any{
		"status": "update_available", "installed": "v0.3.3", "latest": "v0.3.4", "saved_installer": dest,
		"signing_key_id": "702c62788d60920da8ea80aab93ca3e1", "released_at": "2026-10-17T12:00:00Z",
	} {
		if got[k] != want {
			t.Errorf("%s = %v, want %v", k, got[k], want)
		}
	}
	if _, leaked := got["Installer"]; leaked || strings.Contains(out, stub.dirs[0]) {
		t.Errorf("the scratch path leaked into the JSON: %s", out)
	}

	h2 := newFakeHost(t)
	(&updateStub{rep: reportUnknown}).install(h2)
	code, out, _ = runUpdateArgs(t, h2, "check", "--json")
	if code != 1 || strings.Contains(out, `"latest"`) || !strings.Contains(out, `"status":"unknown"`) || !strings.Contains(out, "HTTP 404") {
		t.Errorf("unknown JSON: code=%d %s", code, out)
	}
}

func TestSaveInstallerWritesAPrivateNewFileOnlyFromAVerifiedRelease(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "baft-install-v0.3.4.sh")

	h := newFakeHost(t)
	(&updateStub{rep: reportAvailable}).install(h)
	code, out, _ := runUpdateArgs(t, h, "check", "--save-installer", dest)
	if code != exitUpdateAvailable {
		t.Fatalf("code = %d\n%s", code, out)
	}
	if b, err := os.ReadFile(dest); err != nil || string(b) != stubInstaller {
		t.Fatalf("saved installer = %q, %v", b, err)
	}
	if st, _ := os.Stat(dest); st.Mode().Perm() != 0o700 {
		t.Errorf("mode = %v, want 0700", st.Mode().Perm())
	}
	if !strings.Contains(out, "the verified installer is saved to "+dest) || !strings.Contains(out, "BAFT_VERSION=v0.3.4 bash "+dest+" --plan") {
		t.Errorf("output does not point at the saved installer:\n%s", out)
	}

	// An existing file is never replaced.
	if err := os.WriteFile(dest, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, _ = runUpdateArgs(t, h, "check", "--save-installer", dest)
	if code != 1 || !strings.Contains(out, "The installer was NOT saved") {
		t.Errorf("overwrite: code=%d\n%s", code, out)
	}
	if b, _ := os.ReadFile(dest); string(b) != "mine" {
		t.Error("an existing file was overwritten")
	}
	// Nor is a symlink followed.
	link := filepath.Join(dir, "link")
	if err := os.Symlink(filepath.Join(dir, "elsewhere"), link); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := runUpdateArgs(t, h, "check", "--save-installer", link); code != 1 {
		t.Errorf("symlink: code=%d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "elsewhere")); !os.IsNotExist(err) {
		t.Error("a symlink target was written through")
	}

	// An unverified result has no installer to give.
	h2 := newFakeHost(t)
	(&updateStub{rep: reportUnknown}).install(h2)
	none := filepath.Join(dir, "none.sh")
	code, out, _ = runUpdateArgs(t, h2, "check", "--save-installer", none)
	if code != 1 || !strings.Contains(out, "there is no verified installer to save") {
		t.Errorf("unknown: code=%d\n%s", code, out)
	}
	if _, err := os.Stat(none); !os.IsNotExist(err) {
		t.Error("an installer was saved without a verified release")
	}
}

func TestRealCheckRefusesPlainHTTPSourcesBeforeAnyRequest(t *testing.T) {
	t.Setenv("BAFT_RELEASE_URL", "http://127.0.0.1:1/dl")
	t.Setenv("BAFT_REVOCATIONS_URL", "")
	t.Setenv("BAFT_GITHUB_REPO", "")
	var o, e bytes.Buffer
	code := runUpdate(context.Background(), []string{"check", "--release-state", filepath.Join(t.TempDir(), "none.json")}, strings.NewReader(""), &o, &e, opsEnv{})
	if code != 1 || !strings.Contains(o.String(), "only https is accepted") {
		t.Fatalf("code=%d\n%s%s", code, o.String(), e.String())
	}
}

// ---- the menu ----

func TestMenuUpdateOffersACheckAndAnApplyThatPlansFirst(t *testing.T) {
	r := newMenuRig(t)
	stub := &updateStub{rep: reportAvailable}
	stub.install(r.h)
	before := len(r.commands)
	// Update > check, Enter, Update > apply (not root here, so a plan only), Enter, back, exit.
	r.run(t, monoCaps, "11\n1\n\n2\n\n0\n0\n")
	o := r.out.String()
	for _, w := range []string{"Check for a newer signed release (read-only)", "Apply an update (shows the plan first", "UPDATE AVAILABLE", "v0.3.4",
		"Not running as root, so this shows the plan only", "Nothing to apply"} {
		if !strings.Contains(o, w) {
			t.Errorf("menu output lacks %q:\n%s", w, o)
		}
	}
	if len(stub.calls) != 2 || !strings.HasSuffix(stub.calls[0], "|") {
		t.Errorf("calls = %v", stub.calls)
	}
	for _, d := range stub.dirs {
		if _, err := os.Stat(d); !os.IsNotExist(err) {
			t.Errorf("the scratch directory survived: %v", err)
		}
	}
	for _, c := range r.commands[before:] {
		if !strings.HasPrefix(c, "systemctl is-active") && !strings.HasPrefix(c, "systemctl show") {
			t.Errorf("the update menu ran %q", c)
		}
	}
}

func TestMenuUpdateOnAFailedCheckClaimsNothing(t *testing.T) {
	r := newMenuRig(t)
	(&updateStub{rep: reportUnknown}).install(r.h)
	r.run(t, monoCaps, "11\n1\n\n0\n0\n")
	o := r.out.String()
	if !strings.Contains(o, "UNKNOWN") || !strings.Contains(o, "No claim is made") {
		t.Errorf("a failed check is not reported honestly:\n%s", o)
	}
	for _, bad := range []string{"CURRENT", "UPDATE AVAILABLE", "up to date"} {
		if strings.Contains(o, bad) {
			t.Errorf("a failed check printed %q:\n%s", bad, o)
		}
	}
}

func TestAnOverriddenReleaseRootIsShownInTheReport(t *testing.T) {
	pub, _, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("BAFT_ROOT_PUB", release.EncodePublic(pub))
	t.Setenv("BAFT_RELEASE_URL", "http://127.0.0.1:1/dl") // refused before any request
	t.Setenv("BAFT_REVOCATIONS_URL", "")
	t.Setenv("BAFT_GITHUB_REPO", "")
	var o, e bytes.Buffer
	args := []string{"check", "--json", "--release-state", filepath.Join(t.TempDir(), "none.json")}
	runUpdate(context.Background(), args, strings.NewReader(""), &o, &e, opsEnv{inventory: &inventoryHost{
		readFile: func(string) ([]byte, error) { return nil, os.ErrNotExist }, hashFile: sha256File, newestBackup: newestBackup}})
	if !strings.Contains(o.String(), `"root_overridden":true`) {
		t.Fatalf("json lacks the override:\n%s", o.String())
	}
	o.Reset()
	runUpdate(context.Background(), args[:1], strings.NewReader(""), &o, &e, opsEnv{inventory: &inventoryHost{
		readFile: func(string) ([]byte, error) { return nil, os.ErrNotExist }, hashFile: sha256File, newestBackup: newestBackup}})
	if !strings.Contains(o.String(), "trust root    OVERRIDDEN by BAFT_ROOT_PUB") {
		t.Fatalf("text lacks the override:\n%s", o.String())
	}

	t.Setenv("BAFT_ROOT_PUB", "not-a-key")
	o.Reset()
	if code := runUpdate(context.Background(), []string{"check"}, strings.NewReader(""), &o, &e, opsEnv{inventory: &inventoryHost{
		readFile: func(string) ([]byte, error) { return nil, os.ErrNotExist }, hashFile: sha256File, newestBackup: newestBackup}}); code != 1 || !strings.Contains(o.String(), "release root key is unusable") {
		t.Fatalf("a bad override: code=%d\n%s", code, o.String())
	}
}
