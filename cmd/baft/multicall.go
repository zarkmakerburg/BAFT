package main

import (
	"os"
	"path/filepath"

	"github.com/zarkmakerburg/baft/internal/cli/agentcmd"
	"github.com/zarkmakerburg/baft/internal/cli/bcccmd"
	"github.com/zarkmakerburg/baft/internal/cli/paircmd"
)

// One binary for every BAFT program (ADR-004 §4). baft runs the agent, the
// panel and the pairing tool as `baft agent`, `baft panel` and `baft pair`,
// and also when it is started under their old names (baft-agent, baft-bcc,
// baft-pair: a link or a copy), so existing units, scripts and agents keep
// working unchanged.

// multiCall runs another program when argv0 or the first argument names one.
// It reports whether it did, and that program's exit code.
func multiCall(argv0 string, args []string) (int, bool) {
	switch filepath.Base(argv0) {
	case "baft-agent":
		return runAgent(args), true
	case "baft-bcc":
		return runPanel(args), true
	case "baft-pair":
		return runPair(args), true
	}
	if len(args) > 0 {
		switch args[0] {
		case "agent":
			return runAgent(args[1:]), true
		case "panel":
			return runPanel(args[1:]), true
		case "pair":
			return runPair(args[1:]), true
		}
	}
	return 0, false
}

func runAgent(args []string) int {
	agentcmd.Version = version
	return agentcmd.Main(args)
}

// The panel and the pairing tool read os.Args like a program's main, so they
// see exactly the arguments they would as separate binaries.
func runPanel(args []string) int {
	bcccmd.Version = version
	os.Args = append([]string{"baft-bcc"}, args...)
	bcccmd.Main()
	return 0
}

func runPair(args []string) int {
	os.Args = append([]string{"baft-pair"}, args...)
	paircmd.Main()
	return 0
}
