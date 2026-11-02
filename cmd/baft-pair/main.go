// Command baft-pair creates and applies pairing codes. The same code runs as
// `baft pair` and when the multi-call baft binary is started as baft-pair.
package main

import "github.com/zarkmakerburg/baft/internal/cli/paircmd"

func main() { paircmd.Main() }
