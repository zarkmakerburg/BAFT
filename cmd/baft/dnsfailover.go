package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"

	"github.com/zarkmakerburg/baft/internal/dnsfailover"
)

// runDNSFailover keeps a Cloudflare DNS name on the reachable IR entry
// servers. --once probes a single time, reconciles and prints the result.
func runDNSFailover(ctx context.Context, args []string, stdout, stderr io.Writer) int {
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
