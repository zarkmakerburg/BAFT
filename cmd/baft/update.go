package main

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/zarkmakerburg/baft/internal/release"
	"github.com/zarkmakerburg/baft/internal/selfupdate"
	"github.com/zarkmakerburg/baft/release/keys"
)

// `baft update check`: is a newer signed release out? It only reads. The
// release is verified against the pinned root key, the revocation list and
// this host's release state (no downgrade, no re-tag, no replay) by
// internal/selfupdate; nothing on the host is changed and nothing downloaded
// is run. Applying a release is still the installer's job (install.sh), which
// verifies the binaries again; `--save-installer` hands the operator the
// signed installer of the checked release for that.

const (
	updateCheckTimeout = 90 * time.Second

	exitUpdateAvailable = 3
)

func updateUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: baft update check [--release-state /opt/baft/release-state.json] [--version vX.Y.Z] [--save-installer FILE] [--json]")
	fmt.Fprintln(w, "       baft update apply [--plan] [--yes] [--only transport,agent,bcc] [--version vX.Y.Z] [--skip-backup-check] [--allow-root-override]")
	fmt.Fprintln(w, "  check changes nothing on this host. Exit 0: current, 3: an update is available,")
	fmt.Fprintln(w, "  1: unknown (could not fetch or verify), 2: usage.")
	fmt.Fprintln(w, "  apply runs the release's signed installer for every installed component that is behind")
	fmt.Fprintln(w, "  (BCC, then agent, then the transport service), after showing the plan and asking. Run as root;")
	fmt.Fprintln(w, "  --plan previews without it. It refuses anything that would touch keys, config, pairing or units.")
	fmt.Fprintln(w, "  Honors BAFT_GITHUB_REPO, BAFT_RELEASE_URL and BAFT_REVOCATIONS_URL like install.sh (https only).")
}

func runUpdate(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, env opsEnv) int {
	if len(args) > 0 && args[0] == "apply" {
		return runUpdateApply(ctx, args[1:], stdin, stdout, stderr, env)
	}
	if len(args) == 0 || args[0] != "check" {
		updateUsage(stderr)
		return 2
	}
	var version, saveTo string
	var l menuCfg
	f, ok := parseOpsFlags("update check", args[1:], stderr, func(fs *flag.FlagSet) {
		fs.StringVar(&version, "version", "", "release to check (default: the newest)")
		fs.StringVar(&saveTo, "save-installer", "", "write the verified installer to this new file")
		fs.StringVar(&l.unitDir, "unit-dir", "/etc/systemd/system", "systemd unit directory")
		fs.StringVar(&l.agentUnit, "agent-unit", "baft-agent", "agent service name")
		fs.StringVar(&l.bccService, "bcc-service", "baft-bcc", "BCC service name")
	})
	if !ok {
		updateUsage(stderr)
		return 2
	}
	l.file, l.service, l.releaseState = f.file, f.service, f.releaseState
	rep, comps, saved, saveErr := checkForUpdate(ctx, env, l, version, saveTo)
	if f.json {
		out := struct {
			selfupdate.Report
			Components     []componentInfo `json:"components,omitempty"`
			SavedInstaller string          `json:"saved_installer,omitempty"`
			SaveError      string          `json:"save_error,omitempty"`
		}{Report: rep, Components: comps, SavedInstaller: saved}
		if saveErr != nil {
			out.SaveError = saveErr.Error()
		}
		if err := json.NewEncoder(stdout).Encode(out); err != nil {
			fmt.Fprintln(stderr, "update check:", err)
			return 1
		}
	} else {
		printUpdateReport(stdout, rep, comps, saved, saveErr)
	}
	switch {
	case saveErr != nil || rep.Status == selfupdate.Unknown:
		return 1
	case rep.Status == selfupdate.UpdateAvailable:
		return exitUpdateAvailable
	}
	return 0
}

// checkForUpdate runs one check in a private scratch directory that is removed
// before it returns, then judges each installed component by its own binaries.
// When save is set and the release verified, the signed installer is written
// there (a new file, mode 0700; an existing path is never overwritten).
func checkForUpdate(ctx context.Context, env opsEnv, l menuCfg, version, save string) (rep selfupdate.Report, comps []componentInfo, saved string, saveErr error) {
	ctx, cancel := context.WithTimeout(ctx, updateCheckTimeout)
	defer cancel()
	dir, err := os.MkdirTemp("", "baft-update-")
	if err != nil {
		return selfupdate.Report{Status: selfupdate.Unknown, CheckedAt: time.Now().UTC(), Reason: "cannot create a scratch directory: " + err.Error()}, nil, "", nil
	}
	defer os.RemoveAll(dir)
	check := env.checkUpdate
	if check == nil {
		check = hostUpdateCheck
	}
	rep = check(ctx, l.releaseState, version, dir)
	inv := hostInventory
	if env.inventory != nil {
		inv = *env.inventory
	}
	comps = inspectComponents(l, runtime.GOARCH, rep.Artifacts, inv)
	foldInventory(&rep, comps)
	if save == "" {
		return rep, comps, "", nil
	}
	if rep.Installer == "" {
		return rep, comps, "", fmt.Errorf("there is no verified installer to save")
	}
	body, err := os.ReadFile(rep.Installer)
	if err != nil {
		return rep, comps, "", fmt.Errorf("cannot read the verified installer: %w", err)
	}
	if err := writeNewFile(save, body, 0o700); err != nil {
		return rep, comps, "", fmt.Errorf("cannot save the installer: %w", err)
	}
	return rep, comps, save, nil
}

// hostUpdateCheck is the real check: the pinned root key, this machine's
// architecture and the installer's release-source environment variables.
func hostUpdateCheck(ctx context.Context, statePath, version, dir string) selfupdate.Report {
	root, overridden, err := releaseRoot()
	if err != nil {
		return selfupdate.Report{Status: selfupdate.Unknown, CheckedAt: time.Now().UTC(), RootOverridden: overridden, Reason: "the release root key is unusable: " + err.Error()}
	}
	rep := selfupdate.Check(ctx, selfupdate.Config{
		Root: root,
		Source: selfupdate.Source{
			Repo:           os.Getenv("BAFT_GITHUB_REPO"),
			ReleaseURL:     os.Getenv("BAFT_RELEASE_URL"),
			RevocationsURL: os.Getenv("BAFT_REVOCATIONS_URL"),
		},
		StatePath: statePath,
		Arch:      runtime.GOARCH,
	}, version, dir)
	rep.RootOverridden = overridden
	return rep
}

// releaseRoot is the key a release must chain to: the one pinned in the binary
// (release/keys/root.pub, the file install.sh pins). BAFT_ROOT_PUB overrides
// it exactly as it does for install.sh, for releases signed with throwaway
// keys in a lab; overridden says so, and callers must show it.
func releaseRoot() (key ed25519.PublicKey, overridden bool, err error) {
	if v := strings.TrimSpace(os.Getenv("BAFT_ROOT_PUB")); v != "" {
		key, err = release.DecodePublic(v)
		return key, true, err
	}
	key, err = keys.Root()
	return key, false, err
}

// writeNewFile creates path exclusively (it also refuses a symlink), so a
// saved installer never replaces anything.
func writeNewFile(path string, body []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, err = f.Write(body)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
	}
	return err
}

// printReleaseFacts prints the verified facts of a check: installed and newest
// release, the verdict, and each component's state.
func printReleaseFacts(w io.Writer, rep selfupdate.Report, comps []componentInfo) {
	fmt.Fprintln(w, "BAFT update check")
	installed := "unknown"
	if rep.Installed != "" {
		installed = clean(rep.Installed)
	}
	fmt.Fprintf(w, "  installed     %s\n", installed)
	if rep.Latest != "" {
		fmt.Fprintf(w, "  newest        %s (signed; commit %s; released %s)\n", clean(rep.Latest), shortCommit(rep.Commit), rep.ReleasedAt.Format(time.RFC3339))
		fmt.Fprintf(w, "  signing key   %s, revocation list #%d\n", clean(rep.SigningKeyID), rep.RevocationSequence)
	} else {
		fmt.Fprintln(w, "  newest        unknown")
	}
	if rep.RootOverridden {
		fmt.Fprintln(w, "  trust root    OVERRIDDEN by BAFT_ROOT_PUB (lab and test releases only)")
	}
	fmt.Fprintf(w, "  status        %s\n", strings.ToUpper(strings.ReplaceAll(string(rep.Status), "_", " ")))
	if rep.Reason != "" {
		fmt.Fprintf(w, "  reason        %s\n", clean(rep.Reason))
	}
	fmt.Fprintf(w, "  checked       %s\n", rep.CheckedAt.Format(time.RFC3339))
	if len(comps) > 0 {
		fmt.Fprintln(w, "  components    judged by their own binaries against the signed release")
		for _, c := range comps {
			fmt.Fprintf(w, "    %-10s %-14s %-9s %s\n", c.Name, clean(c.Unit), strings.ToUpper(c.Status), componentDetail(c))
		}
	}
}

func printUpdateReport(w io.Writer, rep selfupdate.Report, comps []componentInfo, saved string, saveErr error) {
	printReleaseFacts(w, rep, comps)
	fmt.Fprintln(w)

	switch rep.Status {
	case selfupdate.Current:
		fmt.Fprintln(w, "The installed release is the newest verified signed release. Nothing was changed.")
	case selfupdate.UpdateAvailable:
		fmt.Fprintln(w, "Nothing on this host was changed. To apply it with the release's signed installer:")
		fmt.Fprintln(w, "  baft update apply --plan    (shows what would change; run it with sudo, a normal user cannot read the root-only files it needs)")
		fmt.Fprintln(w, "  sudo baft update apply      (asks before it changes anything; rolls back on failure)")
		file := "FILE"
		if saved != "" {
			file = saved
			fmt.Fprintf(w, "By hand instead: the verified installer is saved to %s\n", saved)
		} else {
			fmt.Fprintln(w, "By hand instead: baft update check --save-installer FILE (verifies, then writes the signed installer)")
		}
		fmt.Fprintf(w, "  sudo BAFT_VERSION=%s bash %s --plan   (add the options you installed with, such as --agent-only)\n", clean(rep.Latest), file)
		fmt.Fprintln(w, "The installer refuses a downgrade and verifies the binaries again before it changes anything.")
	default:
		fmt.Fprintln(w, "No claim is made about whether an update exists. Nothing was changed.")
	}
	if saveErr != nil {
		fmt.Fprintf(w, "The installer was NOT saved: %s\n", clean(saveErr.Error()))
	}
}

func componentDetail(c componentInfo) string {
	var differs []string
	for _, b := range c.Binaries {
		if b.State == "differs" {
			differs = append(differs, b.Name)
		}
	}
	switch {
	case len(differs) > 0:
		return strings.Join(differs, ", ") + " differs from the signed release"
	case c.Status == statCurrent:
		return "all binaries identical to the signed release"
	}
	return clean(c.Reason)
}

func shortCommit(c string) string {
	if len(c) > 12 {
		return clean(c[:12])
	}
	return clean(c)
}
