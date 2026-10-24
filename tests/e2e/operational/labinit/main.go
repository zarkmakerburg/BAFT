//go:build linux

// Command labinit is a minimal LAB-only PID 1 for the container variant of the
// operational-ingress-v1 gate. The product's inventory deliberately supports
// only a host whose PID 1 is "systemd" and whose network namespace holds a
// single process in a private namespace (see ingressinventory). A container
// has neither, so this stub is built and installed as .../systemd, creates a
// private network namespace with only lo (UP), and runs the given command in
// it. It is never used by the GitHub Actions gate, which runs on a real
// systemd host with `ip netns exec`.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--in-netns" {
		fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, 0)
		if err == nil {
			ifr, e := unix.NewIfreq("lo")
			if e == nil {
				_ = unix.IoctlIfreq(fd, unix.SIOCGIFFLAGS, ifr)
				ifr.SetUint16(ifr.Uint16() | unix.IFF_UP)
				err = unix.IoctlIfreq(fd, unix.SIOCSIFFLAGS, ifr)
			}
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "labinit: lo up:", err)
			os.Exit(125)
		}
		bin, err := exec.LookPath(os.Args[2])
		if err != nil {
			fmt.Fprintln(os.Stderr, "labinit:", err)
			os.Exit(127)
		}
		fmt.Fprintln(os.Stderr, "labinit:", syscall.Exec(bin, os.Args[2:], os.Environ()))
		os.Exit(126)
	}
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: systemd <command> [args...]")
		os.Exit(2)
	}
	cmd := exec.Command("/proc/self/exe", append([]string{"--in-netns"}, os.Args[1:]...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Cloneflags: syscall.CLONE_NEWNET}
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			os.Exit(ee.ExitCode())
		}
		fmt.Fprintln(os.Stderr, "labinit:", err)
		os.Exit(125)
	}
}
