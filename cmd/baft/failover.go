package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
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
	if len(args) > 0 && args[0] == "install" {
		return installService("baft-failover", "BAFT EX failover (IR side)", "failover", args[1:], func(a []string) error {
			return validateFailoverArgs(a, stderr)
		}, "/etc/systemd/system", runSystemctl, os.Stdout, stderr)
	}
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
	if len(args) > 0 && args[0] == "install" {
		return installService("baft-echo", "BAFT failover probe echo (EX side)", "echo", args[1:], func(a []string) error {
			fs := flag.NewFlagSet("echo", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			l := fs.String("listen", "127.0.0.1:2444", "")
			if err := fs.Parse(a); err != nil || fs.NArg() != 0 {
				return fmt.Errorf("usage: baft echo install [--listen 127.0.0.1:2444]")
			}
			if h, _, err := net.SplitHostPort(*l); err != nil || !net.ParseIP(h).IsLoopback() {
				return fmt.Errorf("echo --listen must be a loopback HOST:PORT (it is reached through the tunnel)")
			}
			return nil
		}, "/etc/systemd/system", runSystemctl, os.Stdout, stderr)
	}
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

// validateFailoverArgs parses the run flags without starting anything.
func validateFailoverArgs(args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("failover", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	listen := fs.String("listen", "", "")
	var targets failoverTargets
	fs.Var(&targets, "target", "")
	fs.Duration("probe-interval", 0, "")
	fs.Duration("probe-timeout", 0, "")
	fs.Int("probe-bytes", 0, "")
	fs.Int("fail-after", 0, "")
	fs.Int("recover-after", 0, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *listen == "" || len(targets) < 2 || fs.NArg() != 0 {
		return fmt.Errorf("usage: baft failover install --listen 127.0.0.1:PORT --target ex1=ROUTE[,probe=ECHO] --target ex2=...")
	}
	return nil
}

const baftServiceUnit = `[Unit]
Description=%s
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=%s
Restart=always
RestartSec=5
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes

[Install]
WantedBy=multi-user.target
`

// installService validates the run arguments, writes a systemd unit that runs
// "baft <sub> <args>" and enables and starts it.
func installService(unitName, desc, sub string, args []string, validate func([]string) error, unitDir string, systemctl func(...string) error, stdout, stderr io.Writer) int {
	if err := validate(args); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	exe, err := os.Executable()
	if err != nil {
		exe = "/usr/local/bin/baft"
	}
	parts := []string{exe, sub}
	for _, a := range args {
		if !safeUnitArg(a) {
			fmt.Fprintf(stderr, "argument %q contains characters not allowed in a unit file\n", a)
			return 2
		}
		parts = append(parts, a)
	}
	unit := unitDir + "/" + unitName + ".service"
	if err := writeFileAtomic(unit, []byte(fmt.Sprintf(baftServiceUnit, desc, strings.Join(parts, " "))), 0o644); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	for _, a := range [][]string{{"daemon-reload"}, {"enable", "--now", unitName + ".service"}, {"restart", unitName + ".service"}} {
		if err := systemctl(a...); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	fmt.Fprintf(stdout, "%s installed and started (%s)\n", unitName, unit)
	return 0
}

// safeUnitArg allows only characters that need no quoting in ExecStart.
func safeUnitArg(a string) bool {
	if a == "" {
		return false
	}
	for _, r := range a {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._:,=/[]-", r)
		if !ok {
			return false
		}
	}
	return true
}
