//go:build linux

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestMenuSelectionPTYHelper(t *testing.T) {
	if os.Getenv("BAFT_GO_MENU_PTY_CHILD") != "1" {
		return
	}
	before, err := unix.IoctlGetTermios(int(os.Stdin.Fd()), unix.TCGETS)
	if err != nil {
		t.Fatal(err)
	}
	r := newMenuRig(t)
	code := runMenu([]string{"--file", r.cfg, "--unit-dir", r.dir + "/units", "--state-dir", r.dir + "/state"}, os.Stdin, os.Stdout, os.Stderr, r.h.env, detectTerm(os.Getenv, true, 80), time.Now)
	after, err := unix.IoctlGetTermios(int(os.Stdin.Fd()), unix.TCGETS)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("\nMENU_DONE:%d:RESTORED:%t\n", code, reflect.DeepEqual(before, after))
}

func TestMenuSelectionRealPTY(t *testing.T) {
	for _, tc := range []struct {
		name, term, keys, want string
		signal                 bool
	}{
		{name: "tab enters real submenu", term: "xterm", keys: "\t\n0\n\n0\n", want: "Run doctor"},
		{name: "down enters real submenu", term: "xterm", keys: "\x1b[B\n0\n\n0\n", want: "Run doctor"},
		{name: "up wraps to exit", term: "xterm", keys: "\x1b[A\n", want: "Up/Down"},
		{name: "application down enters submenu", term: "xterm", keys: "\x1bOB\n0\n\n0\n", want: "Run doctor"},
		{name: "application up wraps exit", term: "xterm", keys: "\x1bOA\n", want: "Up/Down"},
		{name: "shift tab wraps to exit", term: "xterm", keys: "\x1b[Z\n", want: "Tab/Shift+Tab"},
		{name: "number and cooked pause", term: "xterm", keys: "1\n\n0\n", want: "baft: active"},
		{name: "multidigit and back", term: "xterm", keys: "14\nback\n\n0\n", want: "Version / Build metadata"},
		{name: "exit from submenu", term: "xterm", keys: "2\nexit\n", want: "Run doctor"},
		{name: "ctrl c restores terminal", term: "xterm", keys: "\x03", want: "Tab/Shift+Tab"},
		{name: "termination restores terminal", term: "xterm", signal: true, want: "Tab/Shift+Tab"},
		{name: "dumb numbered fallback", term: "dumb", keys: "0\n", want: "Overview / Status"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			master, e := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
			if e != nil {
				t.Skip(e)
			}
			defer master.Close()
			if e = unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); e != nil {
				t.Fatal(e)
			}
			number, e := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
			if e != nil {
				t.Fatal(e)
			}
			slave, e := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR, 0)
			if e != nil {
				t.Fatal(e)
			}
			defer slave.Close()
			command := exec.Command(os.Args[0], "-test.run=^TestMenuSelectionPTYHelper$", "-test.count=1")
			command.Env = append(os.Environ(), "BAFT_GO_MENU_PTY_CHILD=1", "TERM="+tc.term, "NO_COLOR=1")
			command.Stdin = slave
			command.Stdout = slave
			command.Stderr = slave
			if e = command.Start(); e != nil {
				t.Fatal(e)
			}
			defer func() { _ = command.Process.Kill(); _ = command.Wait() }()
			var output bytes.Buffer
			deadline := time.Now().Add(8 * time.Second)
			sent := false
			for time.Now().Before(deadline) {
				fds := []unix.PollFd{{Fd: int32(master.Fd()), Events: unix.POLLIN}}
				n, e := unix.Poll(fds, 100)
				if e == unix.EINTR {
					continue
				}
				if e != nil {
					t.Fatal(e)
				}
				if n == 0 {
					continue
				}
				data := make([]byte, 8192)
				n, e = master.Read(data)
				if e != nil {
					break
				}
				output.Write(data[:n])
				prompt := "baft> ["
				if tc.term == "dumb" {
					prompt = "baft> "
				}
				if !sent && strings.Contains(output.String(), prompt) {
					if tc.signal {
						e = command.Process.Signal(syscall.SIGTERM)
					} else {
						_, e = master.Write([]byte(tc.keys))
					}
					if e != nil {
						t.Fatal(e)
					}
					sent = true
				}
				if strings.Contains(output.String(), "MENU_DONE:") {
					break
				}
			}
			text := output.String()
			if !sent || !strings.Contains(text, "MENU_DONE:0:RESTORED:true") || !strings.Contains(text, tc.want) {
				t.Fatalf("selection failed:\n%s", text)
			}
			if tc.term == "dumb" && (strings.Contains(text, "\x1b") || strings.Contains(text, "Tab/Shift+Tab")) {
				t.Fatalf("terminal escapes in dumb fallback: %q", text)
			}
			if tc.name == "exit from submenu" && strings.Contains(text, "Press Enter to return") {
				t.Fatal("exit paused unexpectedly")
			}
		})
	}
}
