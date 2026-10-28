//go:build !linux

package oplab

import "testing"

// Main just runs the tests: the LAB exists only on Linux.
func Main(m interface{ Run() int }) int { return m.Run() }

// Require skips: the operational ingress LAB is Linux-only.
func Require(t testing.TB) { t.Helper(); t.Skip("operational ingress LAB is Linux only") }

// RequireNetns skips: the operational ingress LAB is Linux-only.
func RequireNetns(t testing.TB) { t.Helper(); t.Skip("operational ingress LAB is Linux only") }

// IPBinary is unavailable off Linux.
func IPBinary() string { return "" }
