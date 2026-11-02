// Command baft-agent runs the BCC agent. The same code runs as `baft agent`
// and when the multi-call baft binary is started under the name baft-agent.
package main

import (
	"os"

	"github.com/zarkmakerburg/baft/internal/cli/agentcmd"
)

// version is set at release build time (-X main.version).
var version = "dev"

func main() {
	agentcmd.Version = version
	os.Exit(agentcmd.Main(os.Args[1:]))
}
