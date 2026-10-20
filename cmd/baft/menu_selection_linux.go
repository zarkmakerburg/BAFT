//go:build linux

package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
)

func (m *menu) terminalChoice(items []menuItem, back string) (string, bool, bool) {
	fd := int(m.terminal.Fd())
	old, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return "", false, false
	}
	next := *old
	next.Lflag &^= unix.ICANON | unix.ECHO | unix.ISIG
	next.Cc[unix.VMIN] = 1
	next.Cc[unix.VTIME] = 0
	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	defer signal.Stop(interrupted)
	if err = unix.IoctlSetTermios(fd, unix.TCSETS, &next); err != nil {
		return "", false, false
	}
	defer unix.IoctlSetTermios(fd, unix.TCSETS, old)
	defer fmt.Fprintln(m.out)
	fmt.Fprintln(m.out, "\nTab/Shift+Tab: select; number + Enter; 0: "+back)
	s := newMenuSelection(items)
	m.drawSelection(s, items, back)
	for {
		select {
		case <-interrupted:
			return "exit", true, true
		default:
		}
		if m.in.Buffered() == 0 {
			fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
			n, e := unix.Poll(fds, 100)
			if e == unix.EINTR {
				continue
			}
			if e != nil {
				return "", false, true
			}
			if n == 0 {
				s.escape = 0
				continue
			}
			if fds[0].Revents&unix.POLLIN == 0 {
				return "", false, true
			}
		}
		key, e := m.in.ReadByte()
		if e != nil {
			return "", false, true
		}
		if answer, done := s.feed(key); done {
			return answer, true, true
		}
		m.drawSelection(s, items, back)
	}
}
