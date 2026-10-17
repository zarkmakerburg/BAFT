package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

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
	fmt.Fprintln(w, "  Checks the newest signed release; changes nothing on this host. Exit 0: current,")
	fmt.Fprintln(w, "  3: an update is available, 1: unknown (could not fetch or verify), 2: usage.")
	fmt.Fprintln(w, "  Applying an update is not built into this command yet; --save-installer writes the")
	fmt.Fprintln(w, "  verified installer so it can be previewed (--plan) and run.")
	fmt.Fprintln(w, "  Honors BAFT_GITHUB_REPO, BAFT_RELEASE_URL and BAFT_REVOCATIONS_URL like install.sh (https only).")
}

func runUpdate(ctx context.Context, args []string, stdout, stderr io.Writer, env opsEnv) int {
	if len(args) == 0 || args[0] != "check" {
		updateUsage(stderr)
		return 2
	}
	var version, saveTo string
	f, ok := parseOpsFlags("update check", args[1:], stderr, func(fs *flag.FlagSet) {
		fs.StringVar(&version, "version", "", "release to check (default: the newest)")
		fs.StringVar(&saveTo, "save-installer", "", "write the verified installer to this new file")
	})
	if !ok {
		updateUsage(stderr)
		return 2
	}
	rep, saved, saveErr := checkForUpdate(ctx, env, f.releaseState, version, saveTo)
	if f.json {
		out := struct {
			selfupdate.Report
			SavedInstaller string `json:"saved_installer,omitempty"`
			SaveError      string `json:"save_error,omitempty"`
		}{Report: rep, SavedInstaller: saved}
		if saveErr != nil {
			out.SaveError = saveErr.Error()
		}
		if err := json.NewEncoder(stdout).Encode(out); err != nil {
			fmt.Fprintln(stderr, "update check:", err)
			return 1
		}
	} else {
		printUpdateReport(stdout, rep, saved, saveErr)
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
// before it returns. When save is set and the release verified, the signed
// installer is written there (a new file, mode 0700; an existing path is
// never overwritten).
func checkForUpdate(ctx context.Context, env opsEnv, statePath, version, save string) (rep selfupdate.Report, saved string, saveErr error) {
	ctx, cancel := context.WithTimeout(ctx, updateCheckTimeout)
	defer cancel()
	dir, err := os.MkdirTemp("", "baft-update-")
	if err != nil {
		return selfupdate.Report{Status: selfupdate.Unknown, CheckedAt: time.Now().UTC(), Reason: "cannot create a scratch directory: " + err.Error()}, "", nil
	}
	defer os.RemoveAll(dir)
	check := env.checkUpdate
	if check == nil {
		check = hostUpdateCheck
	}
	rep = check(ctx, statePath, version, dir)
	if save == "" {
		return rep, "", nil
	}
	if rep.Installer == "" {
		return rep, "", fmt.Errorf("there is no verified installer to save")
	}
	body, err := os.ReadFile(rep.Installer)
	if err != nil {
		return rep, "", fmt.Errorf("cannot read the verified installer: %w", err)
	}
	if err := writeNewFile(save, body, 0o700); err != nil {
		return rep, "", fmt.Errorf("cannot save the installer: %w", err)
	}
	return rep, save, nil
}

// hostUpdateCheck is the real check: the pinned root key, this machine's
// architecture and the installer's release-source environment variables.
func hostUpdateCheck(ctx context.Context, statePath, version, dir string) selfupdate.Report {
	root, err := keys.Root()
	if err != nil {
		return selfupdate.Report{Status: selfupdate.Unknown, CheckedAt: time.Now().UTC(), Reason: "the pinned release root key is unusable: " + err.Error()}
	}
	return selfupdate.Check(ctx, selfupdate.Config{
		Root: root,
		Source: selfupdate.Source{
			Repo:           os.Getenv("BAFT_GITHUB_REPO"),
			ReleaseURL:     os.Getenv("BAFT_RELEASE_URL"),
			RevocationsURL: os.Getenv("BAFT_REVOCATIONS_URL"),
		},
		StatePath: statePath,
		Arch:      runtime.GOARCH,
	}, version, dir)
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

func printUpdateReport(w io.Writer, rep selfupdate.Report, saved string, saveErr error) {
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
	fmt.Fprintf(w, "  status        %s\n", strings.ToUpper(strings.ReplaceAll(string(rep.Status), "_", " ")))
	if rep.Reason != "" {
		fmt.Fprintf(w, "  reason        %s\n", clean(rep.Reason))
	}
	fmt.Fprintf(w, "  checked       %s\n\n", rep.CheckedAt.Format(time.RFC3339))

	switch rep.Status {
	case selfupdate.Current:
		fmt.Fprintln(w, "The installed release is the newest verified signed release. Nothing was changed.")
	case selfupdate.UpdateAvailable:
		fmt.Fprintln(w, "Nothing on this host was changed. This command does not apply updates; to apply it with the signed installer:")
		file := "FILE"
		if saved != "" {
			file = saved
			fmt.Fprintf(w, "  verified installer saved to %s\n", saved)
		} else {
			fmt.Fprintln(w, "  1. baft update check --save-installer FILE    (verifies, then writes the signed installer)")
		}
		fmt.Fprintf(w, "  2. sudo BAFT_VERSION=%s bash %s --plan   (preview; add the options you installed with, such as --agent-only)\n", clean(rep.Latest), file)
		fmt.Fprintf(w, "  3. sudo BAFT_VERSION=%s bash %s          (asks before it changes anything)\n", clean(rep.Latest), file)
		fmt.Fprintln(w, "The installer refuses a downgrade and verifies the binaries again before it changes anything.")
	default:
		fmt.Fprintln(w, "No claim is made about whether an update exists. Nothing was changed.")
	}
	if saveErr != nil {
		fmt.Fprintf(w, "The installer was NOT saved: %s\n", clean(saveErr.Error()))
	}
}

func shortCommit(c string) string {
	if len(c) > 12 {
		return clean(c[:12])
	}
	return clean(c)
}
