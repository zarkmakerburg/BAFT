//go:build !linux

package main

// Linux is the managed server platform. Other systems keep numbered line input.
func (m *menu) terminalChoice([]menuItem, string) (string, bool, bool) { return "", false, false }
