package main

import (
	"strings"
	"testing"
)

func TestMenuSelectionKeys(t *testing.T) {
	items := []menuItem{{label: "one", run: func(*menu) {}}, {label: "disabled", planned: true}, {label: "three", run: func(*menu) {}}}
	for _, tc := range []struct{ name, keys, want string }{
		{"default", "\n", "1"}, {"tab skips disabled", "\t\n", "3"}, {"shift tab wraps", "\x1b[Z\n", "0"},
		{"tab wraps", "\t\t\t\n", "1"}, {"number overrides tab", "\t1\n", "1"}, {"backspace", "13\x7f\n", "1"},
		{"disabled never dispatched", "2\n3\n", "3"}, {"invalid never dispatched", "99\n3\n", "3"},
		{"back", "back\n", "back"}, {"exit", "exit\n", "exit"}, {"interrupt", "\x03", "exit"}, {"EOF key", "\x04", "exit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newMenuSelection(items)
			for _, key := range []byte(tc.keys) {
				if got, done := s.feed(key); done {
					if got != tc.want {
						t.Fatalf("got %q want %q", got, tc.want)
					}
					return
				}
			}
			t.Fatal("no selection")
		})
	}
}

func TestMenuSubmenuExitUnwindsWithoutPause(t *testing.T) {
	r := newMenuRig(t)
	r.run(t, monoCaps, "2\nexit\n")
	if strings.Contains(r.out.String(), "Press Enter to return") {
		t.Fatal("exit paused after leaving submenu")
	}
}

func TestMenuSelectionPipedReaderUsesPlainLines(t *testing.T) {
	r := newMenuRig(t)
	r.run(t, monoCaps, "2\n0\n\n0\n")
	if strings.Contains(r.out.String(), "Tab/Shift+Tab") {
		t.Fatal("enabled terminal key mode for a piped reader")
	}
}
