//go:build linux

// Command labdiag prints, read-only, why the product's systemd socket-source
// inventory accepts or rejects the host's unit roots. It is a LAB diagnostic
// used by tests/release/opgate/run_lab.sh; it changes nothing.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/zarkmakerburg/baft/internal/ingressinventory"
)

func main() {
	for _, root := range []string{"/etc/systemd/system", "/usr/lib/systemd/system"} {
		fi, err := os.Lstat(root)
		if err != nil {
			fmt.Printf("labdiag: %s: %v\n", root, err)
			continue
		}
		fmt.Printf("labdiag: %s mode=%v\n", root, fi.Mode())
		_, err = ingressinventory.InspectSystemdSocketUnits(root)
		fmt.Printf("labdiag: InspectSystemdSocketUnits(%s) err=%v\n", root, err)
		ents, _ := os.ReadDir(root)
		n := 0
		for _, e := range ents {
			if !strings.HasSuffix(e.Name(), ".socket") && !strings.HasSuffix(e.Name(), ".socket.d") {
				continue
			}
			n++
			info, err := os.Lstat(root + "/" + e.Name())
			if err != nil || !(info.Mode().IsRegular() || info.IsDir()) || info.Mode().Perm()&0022 != 0 {
				fmt.Printf("labdiag:   odd entry %s: %v %v\n", e.Name(), info.Mode(), err)
			}
		}
		fmt.Printf("labdiag: %s has %d .socket/.socket.d entries of %d\n", root, n, len(ents))
	}
}
