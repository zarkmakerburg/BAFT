package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/zarkmakerburg/baft/internal/publicingress"
)

// runIngress is an explicit foreground TCP runtime, not an installer or BCC
// admission path. It makes no firewall, ownership or deployment health claim.
func runIngress(ctx context.Context, args []string, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "run" {
		fmt.Fprintln(stderr, "usage: baft ingress run --listen IP:PORT --target LOOPBACK_IP:PORT [--max-clients 128] [--dial-timeout 3s]")
		return 2
	}
	fs := flag.NewFlagSet("ingress run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: baft ingress run --listen IP:PORT --target LOOPBACK_IP:PORT [flags]")
		fmt.Fprintln(stderr, "Manual TCP runtime: independently verify the intended IR route and custom SSH/management ports. No ownership, firewall or readiness verification is performed.")
		fs.PrintDefaults()
	}
	listen := fs.String("listen", "", "literal listen IP and unprivileged port (2053 is protected)")
	target := fs.String("target", "", "literal loopback target IP and port")
	maxClients := fs.Int("max-clients", 128, "maximum concurrent clients (1..1024)")
	timeout := fs.Duration("dial-timeout", 3*time.Second, "target dial timeout (10ms..10s)")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "unexpected ingress arguments")
		return 2
	}
	forwarder, err := publicingress.New(publicingress.Config{Listen: *listen, Target: *target, MaxClients: *maxClients, DialTimeout: *timeout})
	if err != nil {
		fmt.Fprintln(stderr, "ingress:", err)
		return 2
	}
	if err := forwarder.Run(ctx); err != nil {
		fmt.Fprintln(stderr, "ingress:", err)
		return 1
	}
	return 0
}
