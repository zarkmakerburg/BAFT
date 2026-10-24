//go:build linux

package operational

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

// bannerServer is a foreign service that proves it is alive and untouched by
// answering every connection with a banner.
func bannerServer(t *testing.T, ln net.Listener, banner string) {
	t.Helper()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_, _ = io.WriteString(c, banner)
			_ = c.Close()
		}
	}()
}

func readBanner(addr string) (string, error) {
	c, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return "", err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	b, err := io.ReadAll(c)
	return string(b), err
}

// blockedApply runs the honest signed Apply against a host with a foreign
// claim on the port and asserts the product refused it without touching
// anything of the foreign owner.
func (f *fullPath) blockedApply(t *testing.T) {
	t.Helper()
	f.apply2(t)
}

// apply2 is apply() whose Agent result is expected to be a refusal.
func (f *fullPath) apply2(t *testing.T) {
	t.Helper()
	f.cmd = f.b.stageFor(f.tn, f.job, 2*time.Minute)
	f.a.openGate()
	if err := f.b.store.EnqueueOperationalIngress(f.tn.ID); err != nil {
		t.Fatal(err)
	}
	res := f.a.runOnce()
	if !isFailure(res) {
		t.Fatalf("Apply over a foreign claim was not refused: %+v", res)
	}
}

func (f *fullPath) assertForeignRefusal(t *testing.T) {
	t.Helper()
	if n := f.h.baftRules(); n != 0 {
		t.Fatalf("a rule was installed over a foreign claim: %s", f.h.chainJSON())
	}
	if f.echo.seen() != 0 {
		t.Fatal("traffic reached the tunnel target through a refused ingress")
	}
	op := f.b.op(f.tn.ID)
	if op.Receipt != nil && (op.Receipt.Phase == "applied" || op.Receipt.OwnershipVerified) {
		t.Fatalf("BCC holds an applied receipt for a refused apply: %+v", op.Receipt)
	}
	if v := f.b.readiness(f.tn.ID); v.Ready || v.Card != nil {
		t.Fatalf("card for a refused apply: %+v", v)
	}
}

func TestOperationalIngressV1Gate_RefusesActiveForeignService(t *testing.T) {
	for i, tc := range []struct{ name, network, addr string }{
		{"wildcard", "tcp4", "0.0.0.0"},
		{"loopback", "tcp4", "127.0.0.1"},
		{"public alias", "tcp4", irAddr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			port := 54721 + i
			f := setupFullPath(t, port, "127.0.0.1:1443")
			ln := foreignListen(t, tc.network, fmt.Sprintf("%s:%d", tc.addr, port))
			bannerServer(t, ln, "FOREIGN-ACTIVE\n")
			f.apply2(t)
			f.assertForeignRefusal(t)
			// The foreign service was neither stopped nor replaced.
			if b, err := readBanner(ln.Addr().String()); err != nil || b != "FOREIGN-ACTIVE\n" {
				t.Fatalf("foreign service disturbed: %q %v", b, err)
			}
		})
	}
}

func TestOperationalIngressV1Gate_RefusesStoppedForeignService(t *testing.T) {
	for i, listen := range []string{"%d", "0.0.0.0:%d", "[::]:%d", "127.0.0.1:%d"} {
		t.Run(fmt.Sprintf("ListenStream=%s", listen), func(t *testing.T) {
			port := 54731 + i
			f := setupFullPath(t, port, "127.0.0.1:1443")
			name := fmt.Sprintf("foreign-stopped-%d", port)
			stoppedUnit(t, name, fmt.Sprintf(listen, port))
			unit := "/etc/systemd/system/" + name + ".socket"
			want, _ := os.ReadFile(unit)
			f.apply2(t)
			f.assertForeignRefusal(t)
			if got, err := os.ReadFile(unit); err != nil || !bytes.Equal(got, want) {
				t.Fatalf("the foreign (stopped) unit was modified or removed: %v", err)
			}
			if !portFree(port) {
				t.Fatalf("a listener was left on %d next to a stopped foreign service", port)
			}
			// The same refusal on a retry: a stopped service's claim is not
			// forgotten, and a fresh signed job cannot bypass it.
			f.tap.setHook(nil)
			if v := f.b.readiness(f.tn.ID); v.Ready {
				t.Fatalf("ready: %+v", v)
			}
		})
	}
	// Also a stopped unit pair (socket + service) in the vendor tree.
	t.Run("vendor tree", func(t *testing.T) {
		port := 54739
		f := setupFullPath(t, port, "127.0.0.1:1443")
		path := fmt.Sprintf("/usr/lib/systemd/system/vendor-foreign-%d.socket", port)
		if err := os.WriteFile(path, []byte(fmt.Sprintf("[Socket]\nListenStream=%d\n", port)), 0644); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(path) })
		f.apply2(t)
		f.assertForeignRefusal(t)
	})
}

func TestOperationalIngressV1Gate_RefusesForeignV4V6SiblingSocket(t *testing.T) {
	// Before Apply: a foreign socket on the SAME numeric port in the OTHER
	// family (or another IP of the same family) is still a claim on the port.
	for i, tc := range []struct{ name, network, addr string }{
		{"ipv6 wildcard", "tcp6", "[::]"},
		{"ipv6 loopback", "tcp6", "[::1]"},
		{"ipv4 loopback", "tcp4", "127.0.0.1"},
	} {
		t.Run("before/"+tc.name, func(t *testing.T) {
			port := 54741 + i
			f := setupFullPath(t, port, "127.0.0.1:1443")
			ln := foreignListen(t, tc.network, fmt.Sprintf("%s:%d", tc.addr, port))
			bannerServer(t, ln, "FOREIGN-SIBLING\n")
			f.apply2(t)
			f.assertForeignRefusal(t)
			if b, err := readBanner(ln.Addr().String()); err != nil || b != "FOREIGN-SIBLING\n" {
				t.Fatalf("foreign sibling disturbed: %q %v", b, err)
			}
		})
	}
	// After Apply: a sibling that appears later stops OUR ingress (new
	// clients can no longer be admitted on a port someone else also holds)
	// and is never touched; BCC's own probe then stops verifying the card.
	t.Run("after/late v6 sibling", func(t *testing.T) {
		f := setupFullPath(t, 54745, "127.0.0.1:1443")
		f.legit(t)
		ln := foreignListen(t, "tcp6", "[::]:54745")
		bannerServer(t, ln, "FOREIGN-LATE\n")
		deadline := time.Now().Add(10 * time.Second)
		for roundTrip(t, "127.0.0.1:54745", []byte("x")) == nil {
			if time.Now().After(deadline) {
				t.Fatal("ingress kept serving after a foreign sibling took the same numeric port")
			}
			time.Sleep(200 * time.Millisecond)
		}
		if b, err := readBanner(ln.Addr().String()); err != nil || !strings.HasPrefix(b, "FOREIGN-LATE") {
			t.Fatalf("late foreign sibling was disturbed: %q %v", b, err)
		}
		time.Sleep(11 * time.Second) // observation cache
		if v := f.b.readiness(f.tn.ID); v.Ready || v.Card != nil {
			t.Fatalf("the card survived a lost listener: %+v", v)
		}
	})
}
