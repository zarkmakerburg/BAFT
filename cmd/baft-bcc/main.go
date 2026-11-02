// Command baft-bcc runs the BAFT Command Center (panel). The same code runs as
// `baft panel` and when the multi-call baft binary is started as baft-bcc.
package main

import "github.com/zarkmakerburg/baft/internal/cli/bcccmd"

// version is set at release build time (-X main.version).
var version = "dev"

func main() {
	bcccmd.Version = version
	bcccmd.Main()
}
