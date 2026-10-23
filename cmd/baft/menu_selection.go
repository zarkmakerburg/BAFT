package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Only menu selection uses single-key input. Questions and destructive-action
// confirmations keep the normal cooked, echoed terminal input.
func (m *menu) choose(items []menuItem, back string) (answer string, ok bool) {
	defer func() {
		switch strings.ToLower(answer) {
		case "q", "quit", "exit":
			m.exitRequested = true
			answer = "0"
		}
	}()
	if m.terminal != nil {
		if answer, ok, handled := m.terminalChoice(items, back); handled {
			return answer, ok
		}
	}
	return m.ask(m.st.gold("\nbaft> "))
}

type menuSelection struct {
	choices  []int
	selected int
	typed    string
	escape   int
}

func newMenuSelection(items []menuItem) *menuSelection {
	s := &menuSelection{}
	for i, it := range items {
		if !it.planned && it.run != nil {
			s.choices = append(s.choices, i+1)
		}
	}
	s.choices = append(s.choices, 0)
	return s
}

func (s *menuSelection) label(items []menuItem, back string) string {
	n := s.choices[s.selected]
	if n == 0 {
		return back
	}
	return items[n-1].label
}

// feed returns an answer only after Enter (or Ctrl-C/Ctrl-D). Disabled actions
// cannot gain focus, and an invalid typed number never executes an action.
func (s *menuSelection) feed(key byte) (string, bool) {
	if s.escape != 0 {
		if s.escape == 1 && (key == '[' || key == 'O') {
			s.escape = 2
			if key == 'O' {
				s.escape = 3
			}
			return "", false
		}
		if s.escape == 2 || s.escape == 3 {
			switch {
			case key == 'A' || (s.escape == 2 && key == 'Z'):
				s.selected = (s.selected + len(s.choices) - 1) % len(s.choices)
				s.typed = ""
			case key == 'B':
				s.selected = (s.selected + 1) % len(s.choices)
				s.typed = ""
			}
		}
		s.escape = 0
		return "", false
	}
	switch key {
	case 27:
		s.escape = 1
	case '\t':
		s.selected = (s.selected + 1) % len(s.choices)
		s.typed = ""
	case 127, 8:
		if len(s.typed) > 0 {
			s.typed = s.typed[:len(s.typed)-1]
		}
	case 3, 4:
		return "exit", true
	case '\r', '\n':
		a := strings.ToLower(s.typed)
		if a == "" {
			return strconv.Itoa(s.choices[s.selected]), true
		}
		if a == "q" || a == "quit" || a == "exit" || a == "b" || a == "back" {
			return a, true
		}
		for _, n := range s.choices {
			if a == strconv.Itoa(n) {
				return a, true
			}
		}
		s.typed = ""
	default:
		if ((key >= '0' && key <= '9') || (key >= 'a' && key <= 'z') || (key >= 'A' && key <= 'Z')) && len(s.typed) < 16 {
			s.typed += string(key)
		}
	}
	return "", false
}

func (m *menu) drawSelection(s *menuSelection, items []menuItem, back string) {
	fmt.Fprintf(m.out, "\r\x1b[2Kbaft> [%d %s] %s", s.choices[s.selected], s.label(items, back), s.typed)
}

func menuTerminal(in any, out any, caps termCaps) *os.File {
	input, ok := in.(*os.File)
	if !ok {
		return nil
	}
	output, ok := out.(*os.File)
	if !ok {
		return nil
	}
	if !caps.TTY || caps.Color == colorPlain || !isTerminal(input) || !isTerminal(output) {
		return nil
	}
	return input
}
