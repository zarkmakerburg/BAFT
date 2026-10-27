package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zarkmakerburg/baft/internal/dnsfailover"
)

// runDNSFailover keeps a Cloudflare DNS name on the reachable IR entry
// servers. --once probes a single time, reconciles and prints the result.
func runDNSFailover(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "setup" {
		return runDNSFailoverSetup(ctx, args[1:], os.Stdin, stdout, stderr, runSystemctl, "")
	}
	fs := flag.NewFlagSet("dns-failover", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("config", "", "dns-failover JSON config")
	once := fs.Bool("once", false, "probe once, reconcile, print and exit")
	if err := fs.Parse(args); err != nil || *file == "" || fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: baft dns-failover --config <file.json> [--once]")
		return 2
	}
	cfg, err := dnsfailover.LoadConfig(*file)
	if err != nil {
		fmt.Fprintln(stderr, "invalid config:", err)
		return 1
	}
	token, err := dnsfailover.ReadToken(cfg.TokenFile)
	if err != nil {
		fmt.Fprintln(stderr, "token:", err)
		return 1
	}
	logger := log.New(stderr, "dns-failover: ", log.LstdFlags)
	c := dnsfailover.New(cfg, token, nil, logger.Printf)
	if *once {
		c.ProbeOnce(ctx)
		published, err := c.Reconcile(ctx)
		if err != nil {
			fmt.Fprintln(stderr, "reconcile:", err)
			return 1
		}
		_ = json.NewEncoder(stdout).Encode(map[string]any{"healthy": c.Healthy(), "published": published})
		return 0
	}
	if err := c.Run(ctx); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

type targetList []dnsfailover.Target

func (t *targetList) String() string { return fmt.Sprint(*t) }

// Set parses name=IP or name=IP:probe-port.
func (t *targetList) Set(v string) error {
	name, addr, ok := strings.Cut(v, "=")
	if !ok || name == "" {
		return fmt.Errorf("target %q: want name=IP[:port]", v)
	}
	ip, port := addr, ""
	if h, p, err := net.SplitHostPort(addr); err == nil {
		ip, port = h, p
	}
	*t = append(*t, dnsfailover.Target{Name: name, Address: ip, Probe: port})
	return nil
}

func runSystemctl(args ...string) error {
	out, err := exec.Command("systemctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

const dnsFailoverUnit = `[Unit]
Description=BAFT DNS failover (Cloudflare)
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=%s dns-failover --config %s
Restart=always
RestartSec=10
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes

[Install]
WantedBy=multi-user.target
`

// runDNSFailoverSetup takes a Cloudflare API token (on stdin), a record name
// and the IR targets; it finds the zone, checks the token can manage the
// record, writes the token, config and unit, and starts the service.
func runDNSFailoverSetup(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, systemctl func(...string) error, apiBase string) int {
	fs := flag.NewFlagSet("dns-failover setup", flag.ContinueOnError)
	fs.SetOutput(stderr)
	record := fs.String("record", "", "DNS name clients use, e.g. t.example.com")
	probePort := fs.String("probe-port", "443", "default TCP port probed on each target")
	dir := fs.String("config-dir", "/etc/baft", "where the token and config are written")
	unitDir := fs.String("unit-dir", "/etc/systemd/system", "systemd unit directory")
	noStart := fs.Bool("no-start", false, "write files only; do not enable or start the service")
	var targets targetList
	fs.Var(&targets, "target", "IR entry server as name=IP[:port] (repeat)")
	if err := fs.Parse(args); err != nil || *record == "" || fs.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: echo TOKEN | baft dns-failover setup --record t.example.com --target ir-a=IP --target ir-b=IP [--probe-port 443]")
		return 2
	}
	for i := range targets {
		if targets[i].Probe == "" {
			targets[i].Probe = net.JoinHostPort(targets[i].Address, *probePort)
		} else {
			targets[i].Probe = net.JoinHostPort(targets[i].Address, targets[i].Probe)
		}
	}
	raw, err := io.ReadAll(io.LimitReader(stdin, 4096))
	token := strings.TrimSpace(string(raw))
	if err != nil || token == "" {
		fmt.Fprintln(stderr, "give the Cloudflare API token on stdin (needs Zone:Read and DNS:Edit for the zone)")
		return 2
	}
	tokenFile := filepath.Join(*dir, "cloudflare-dns.token")
	cfgFile := filepath.Join(*dir, "dns-failover.json")
	cfg := dnsfailover.Config{Record: strings.Trim(*record, "."), TokenFile: tokenFile, Targets: targets}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	tmp, err := os.CreateTemp(*dir, ".dns-failover-*.json")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = tmp.Write(b)
	_ = tmp.Close()
	defer os.Remove(tmp.Name())
	cfg, err = dnsfailover.LoadConfig(tmp.Name())
	if err != nil {
		fmt.Fprintln(stderr, "invalid setup:", err)
		return 1
	}
	// Prove the token works for this record before anything is installed.
	c := dnsfailover.New(cfg, token, nil, nil)
	if apiBase != "" {
		c.SetAPIBase(apiBase)
	}
	zone, err := c.FindZone(ctx)
	if err != nil {
		fmt.Fprintln(stderr, "cloudflare:", err)
		return 1
	}
	cfg.ZoneID = zone
	b, _ = json.MarshalIndent(cfg, "", "  ")
	if err := writeFileAtomic(tokenFile, []byte(token+"\n"), 0o600); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := writeFileAtomic(cfgFile, append(b, '\n'), 0o600); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	exe, err := os.Executable()
	if err != nil {
		exe = "/usr/local/bin/baft"
	}
	unit := filepath.Join(*unitDir, "baft-dns-failover.service")
	if err := writeFileAtomic(unit, []byte(fmt.Sprintf(dnsFailoverUnit, exe, cfgFile)), 0o644); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if !*noStart {
		for _, a := range [][]string{{"daemon-reload"}, {"enable", "--now", "baft-dns-failover.service"}, {"restart", "baft-dns-failover.service"}} {
			if err := systemctl(a...); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
		}
	}
	fmt.Fprintf(stdout, "dns-failover ready: %s in zone %s, %d targets\n", cfg.Record, zone, len(cfg.Targets))
	return 0
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
