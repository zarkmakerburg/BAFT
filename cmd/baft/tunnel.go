package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zarkmakerburg/baft/internal/tunnelnode"
)

// terminalSystem has no BCC dependency. The manager alone owns its scoped unit.
type terminalSystem struct{}

func (terminalSystem) Systemctl(ctx context.Context, args ...string) (string, error) {
	b, e := exec.CommandContext(ctx, "systemctl", args...).CombinedOutput()
	return strings.TrimSpace(string(b)), e
}
func (terminalSystem) Run(ctx context.Context, name string, args ...string) (string, error) {
	b, e := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return strings.TrimSpace(string(b)), e
}

// runTunnelCLI deliberately requires an explicit instance so that an existing
// default BAFT service can never be modified by an accidental terminal command.
func runTunnelCLI(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: baft tunnel <ex-code|ir-apply|ex-accept|ir-start|health|rollback> --instance ID --metrics-listen 127.0.0.1:PORT --id TXN [flags]")
		return 2
	}
	action := args[0]
	switch action {
	case "ex-code", "ir-apply", "ex-accept", "ir-start", "health", "rollback":
	default:
		fmt.Fprintln(stderr, "unknown tunnel action")
		return 2
	}
	fs := flag.NewFlagSet("tunnel "+action, flag.ContinueOnError)
	fs.SetOutput(stderr)
	instance := fs.String("instance", "", "independent BAFT instance identifier (required)")
	metrics := fs.String("metrics-listen", "", "dedicated loopback metrics socket")
	id := fs.String("id", "", "transaction ID")
	configDir := fs.String("config-dir", "/etc/baft", "BAFT config root")
	stateDir := fs.String("state-dir", "/var/lib/baft", "BAFT state root")
	unitDir := fs.String("unit-dir", "/etc/systemd/system", "systemd unit directory")
	binDir := fs.String("bin-dir", "/usr/local/bin", "BAFT binaries directory")
	serviceUser := fs.String("service-user", "baft", "service Unix account")
	address := fs.String("address", "", "EX public IP or hostname")
	port := fs.Int("port", 0, "EX public port")
	target := fs.String("target", "", "EX local target literal IP:port")
	routeID := fs.String("route-id", "", "route identifier")
	routeListen := fs.String("route-listen", "", "IR local route listener")
	code := fs.String("code", "", "EX pairing descriptor on IR")
	reply := fs.String("reply", "", "IR response on EX")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *instance == "" || *instance == "default" || *metrics == "" || *id == "" {
		fmt.Fprintln(stderr, "explicit non-default --instance, --metrics-listen and --id are required")
		return 2
	}
	for _, v := range []string{*configDir, *stateDir, *unitDir, *binDir} {
		if !filepath.IsAbs(v) || filepath.Clean(v) != v {
			fmt.Fprintln(stderr, "directories must be normalized absolute paths")
			return 2
		}
	}
	manager, err := tunnelnode.New(tunnelnode.Env{ConfigDir: *configDir, StateDir: *stateDir, UnitDir: *unitDir, Service: "baft", User: *serviceUser, BaftBin: filepath.Join(*binDir, "baft"), PairBin: filepath.Join(*binDir, "baft-pair"), System: terminalSystem{}})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	scoped, err := manager.ForInstance(*instance, *metrics)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	var result string
	switch action {
	case "ex-code":
		if *port == 0 || *address == "" || *target == "" || *routeID == "" {
			fmt.Fprintln(stderr, "ex-code requires --address --port --target --route-id")
			return 2
		}
		result, err = scoped.PrepareEX(ctx, *id, tunnelnode.ExParams{PublicAddress: *address, Port: *port, Target: *target, RouteID: *routeID})
	case "ir-apply":
		if *code == "" || *routeListen == "" || *routeID == "" {
			fmt.Fprintln(stderr, "ir-apply requires --code --route-listen --route-id")
			return 2
		}
		result, err = scoped.PrepareIR(ctx, *id, *code, tunnelnode.IRParams{RouteListen: *routeListen, RouteID: *routeID})
	case "ex-accept":
		if *reply == "" {
			fmt.Fprintln(stderr, "ex-accept requires --reply")
			return 2
		}
		result, err = scoped.CommitEX(ctx, *id, *reply)
	case "ir-start":
		result, err = scoped.CommitIR(ctx, *id)
	case "health":
		result, err = scoped.Health(ctx, *id)
	case "rollback":
		result, err = scoped.Rollback(ctx, *id)
	}
	if err != nil {
		fmt.Fprintln(stderr, "tunnel:", err)
		return 1
	}
	// Pairing descriptors and replies are secrets. Only the caller's terminal
	// receives them; never print them through an error or structured log.
	if action == "ex-code" || action == "ir-apply" {
		fmt.Fprintln(stdout, result)
	} else {
		fmt.Fprintln(stdout, strings.TrimSpace(result))
	}
	return 0
}
