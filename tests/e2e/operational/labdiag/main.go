//go:build linux

// Command labdiag prints, read-only on the host, why the product's systemd
// socket-source inventory accepts or rejects the host's unit roots, one unit
// at a time (each unit is copied into a private temp directory and inspected
// there). It is a LAB diagnostic used by tests/release/opgate/run_lab.sh.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zarkmakerburg/baft/internal/ingressinventory"
)

func main() {
	for _, root := range []string{"/etc/systemd/system", "/usr/lib/systemd/system"} {
		_, err := ingressinventory.InspectSystemdSocketUnits(root)
		fmt.Printf("labdiag: %s whole-root err=%v\n", root, err)
		ents, _ := os.ReadDir(root)
		for _, e := range ents {
			if !strings.HasSuffix(e.Name(), ".socket") {
				continue
			}
			tmp, err := os.MkdirTemp("", "labdiag")
			if err != nil {
				fmt.Println("labdiag:", err)
				return
			}
			_ = os.Chmod(tmp, 0755)
			raw, rerr := os.ReadFile(filepath.Join(root, e.Name()))
			if rerr != nil {
				fmt.Printf("labdiag: %s/%s unreadable: %v\n", root, e.Name(), rerr)
				os.RemoveAll(tmp)
				continue
			}
			_ = os.WriteFile(filepath.Join(tmp, e.Name()), raw, 0644)
			if _, ierr := ingressinventory.InspectSystemdSocketUnits(tmp); ierr != nil {
				fmt.Printf("labdiag: %s/%s rejected: %v\n", root, e.Name(), ierr)
				for _, l := range strings.Split(string(raw), "\n") {
					if strings.HasPrefix(l, "Listen") {
						fmt.Printf("labdiag:     %s\n", l)
					}
				}
			}
			os.RemoveAll(tmp)
		}
	}
}
