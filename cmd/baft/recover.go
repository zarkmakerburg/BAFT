package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"github.com/zarkmakerburg/baft/internal/tunnelnode"
)

// runRecover is `baft recover boot`: the boot-time reconciliation of an
// interrupted tunnel transaction (see tunnelnode.RecoverAtBoot for the exact
// evidence rules). It is started by baft-recover.service, ordered before every
// BAFT service, and never starts, stops or restarts a service itself.
//
// Exit status: 0 nothing needed fencing, 1 at least one instance was fenced
// (left untouched for an operator) or could not be reconciled, 2 usage.
func runRecover(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "boot" {
		fmt.Fprintln(stderr, "usage: baft recover boot [--json] [--config-dir D] [--state-dir D] [--unit-dir D] [--bin-dir D] [--service-user U]")
		return 2
	}
	fs := flag.NewFlagSet("recover boot", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "machine-readable report")
	configDir := fs.String("config-dir", "/etc/baft", "BAFT config root")
	stateDir := fs.String("state-dir", "/var/lib/baft", "BAFT state root")
	unitDir := fs.String("unit-dir", "/etc/systemd/system", "systemd unit directory")
	binDir := fs.String("bin-dir", "/usr/local/bin", "BAFT binaries directory")
	service := fs.String("service", "baft", "BAFT systemd service name")
	serviceUser := fs.String("service-user", "baft", "service Unix account")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 {
		return 2
	}
	for _, v := range []string{*configDir, *stateDir, *unitDir, *binDir} {
		if !filepath.IsAbs(v) || filepath.Clean(v) != v {
			fmt.Fprintln(stderr, "directories must be normalized absolute paths")
			return 2
		}
	}
	reports, err := tunnelnode.RecoverAllAtBoot(ctx, tunnelnode.Env{
		ConfigDir: *configDir, StateDir: *stateDir, UnitDir: *unitDir, Service: *service, User: *serviceUser,
		BaftBin: filepath.Join(*binDir, "baft"), PairBin: filepath.Join(*binDir, "baft-pair"), System: terminalSystem{},
	})
	rc := 0
	if err != nil {
		fmt.Fprintln(stderr, "baft recover boot:", err)
		rc = 1
	}
	for _, r := range reports {
		if r.Action == tunnelnode.BootFenced {
			rc = 1
		}
	}
	if *asJSON {
		_ = json.NewEncoder(stdout).Encode(reports)
		return rc
	}
	for _, r := range reports {
		name := r.Instance
		if name == "" {
			name = "default"
		}
		line := fmt.Sprintf("baft recover boot: instance=%s action=%s", name, r.Action)
		if r.TunnelID != "" {
			line += " change=" + r.TunnelID
		}
		fmt.Fprintf(stdout, "%s: %s\n", line, r.Reason)
	}
	if len(reports) == 0 {
		fmt.Fprintln(stdout, "baft recover boot: nothing to reconcile")
	}
	return rc
}
