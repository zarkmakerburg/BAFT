package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"time"

	"github.com/zarkmakerburg/baft/internal/failover"
)

type failoverTargets []failover.Target

func (t *failoverTargets) String() string { return fmt.Sprint(*t) }

// Set parses name=ADDR or name=ADDR,probe=PROBE_ADDR.
func (t *failoverTargets) Set(v string) error {
	name, rest, ok := strings.Cut(v, "=")
	if !ok || name == "" || rest == "" {
		return fmt.Errorf("target %q: want name=HOST:PORT[,probe=HOST:PORT]", v)
	}
	addr, probe, _ := strings.Cut(rest, ",")
	tg := failover.Target{Name: name, Address: addr}
	if probe != "" {
		p, ok := strings.CutPrefix(probe, "probe=")
		if !ok || p == "" {
			return fmt.Errorf("target %q: want probe=HOST:PORT", v)
		}
		tg.Probe = p
	}
	for _, a := range []string{tg.Address, tg.Probe} {
		if a == "" {
			continue
		}
		if _, _, err := net.SplitHostPort(a); err != nil {
			return fmt.Errorf("target %q: %q is not HOST:PORT", v, a)
		}
	}
	*t = append(*t, tg)
	return nil
}

// runFailover listens locally and forwards each connection to the first
// healthy target (one tunnel route per EX), probing targets end to end.
func runFailover(ctx context.Context, args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("failover", flag.ContinueOnError)
	fs.SetOutput(stderr)
	listen := fs.String("listen", "", "local listen address, e.g. 127.0.0.1:1080")
	var targets failoverTargets
	fs.Var(&targets, "target", "name=ROUTE_ADDR[,probe=ECHO_ROUTE_ADDR] in preference order (repeat)")
	interval := fs.Duration("probe-interval", 5*time.Second, "time between probes")
	timeout := fs.Duration("probe-timeout", 5*time.Second, "time allowed for one probe")
	bytes := fs.Int("probe-bytes", 32<<10, "bytes each probe sends and expects back")
	failAfter := fs.Int("fail-after", 2, "failed probes in a row before a target is skipped")
	recoverAfter := fs.Int("recover-after", 3, "good probes in a row before a target is preferred again")
	if err := fs.Parse(args); err != nil || *listen == "" || len(targets) < 2 || fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: baft failover --listen 127.0.0.1:PORT --target ex1=127.0.0.1:P1,probe=127.0.0.1:Q1 --target ex2=127.0.0.1:P2,probe=127.0.0.1:Q2")
		return 2
	}
	logger := log.New(stderr, "failover: ", log.LstdFlags)
	p := &failover.Proxy{Listen: *listen, Targets: targets, Health: failover.HealthConfig{
		Interval: *interval, Timeout: *timeout, Bytes: *bytes, FailAfter: *failAfter, RecoverAfter: *recoverAfter,
		OnChange: func(name string, ok bool) {
			logger.Printf("target %s is now %s", name, map[bool]string{true: "healthy", false: "down"}[ok])
		}}}
	if err := p.Run(ctx); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// runEcho serves the probe endpoint on the EX side (behind a carried port).
func runEcho(ctx context.Context, args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("echo", flag.ContinueOnError)
	fs.SetOutput(stderr)
	listen := fs.String("listen", "127.0.0.1:2444", "listen address for the failover probe echo")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: baft echo [--listen 127.0.0.1:2444]")
		return 2
	}
	if err := failover.ServeEcho(ctx, *listen); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
