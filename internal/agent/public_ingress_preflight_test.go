package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/release"
)

func ingressPreflightJob() agentjob.Job {
	now := time.Now().UTC().Truncate(time.Second)
	return agentjob.Job{SchemaVersion: agentjob.SchemaVersion, JobID: "ingress-check", NodeID: "ir-test", Action: agentjob.ActionPublicIngressPreflight, Params: map[string]string{"tunnel_id": "tun-test", "public_port": "2053", "local_target": "127.0.0.1:1443"}, IssuedAt: now, ExpiresAt: now.Add(time.Minute)}
}

const ingressSocketHeader = "  sl  local_address rem_address   st\n"

func TestPublicIngressPreflightReadOnlyEvidence(t *testing.T) {
	j := ingressPreflightJob()
	reads := []string{}
	ev, err := collectPublicIngressPreflight(context.Background(), j, func(path string) ([]byte, error) {
		reads = append(reads, path)
		if path == "/proc/net/tcp" {
			return []byte(ingressSocketHeader + "0: 00000000:0805 00000000:0000 0A\n1: 0100007F:05A3 00000000:0000 0A\n"), nil
		}
		return []byte(ingressSocketHeader), nil
	}, func(tool string) (string, error) {
		if tool == "nft" {
			return "/usr/sbin/nft", nil
		}
		return "", errors.New("absent")
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ev.SocketInventoryComplete || ev.PublicPortState != "occupied" || ev.PublicListenerCount != 1 || !ev.TargetListenerObserved {
		t.Fatalf("incorrect observation: %+v", ev)
	}
	if ev.ReadyToApply || ev.OwnershipVerified || ev.FirewallPolicyVerified {
		t.Fatalf("unsupported readiness claim: %+v", ev)
	}
	if strings.Join(reads, ",") != "/proc/net/tcp,/proc/net/tcp6" || strings.Join(ev.FirewallTools, ",") != "nft" {
		t.Fatalf("unexpected discovery: %+v %+v", reads, ev)
	}
}

func TestPublicIngressPreflightUnavailableDoesNotMeanFree(t *testing.T) {
	for _, bad := range []string{"unavailable", "malformed", "oversized"} {
		t.Run(bad, func(t *testing.T) {
			ev, err := collectPublicIngressPreflight(context.Background(), ingressPreflightJob(), func(path string) ([]byte, error) {
				if path == "/proc/net/tcp6" {
					return []byte(ingressSocketHeader), nil
				}
				switch bad {
				case "unavailable":
					return nil, errors.New("denied")
				case "malformed":
					return []byte(ingressSocketHeader + "0: corrupt\n"), nil
				default:
					return nil, fmt.Errorf("socket table exceeds inspection limit")
				}
			}, func(string) (string, error) { return "", errors.New("absent") })
			if err != nil {
				t.Fatal(err)
			}
			if ev.SocketInventoryComplete || ev.PublicPortState != "unknown" || ev.TargetListenerObserved || ev.ReadyToApply {
				t.Fatalf("incomplete inspection accepted: %+v", ev)
			}
		})
	}
	ev, err := collectPublicIngressPreflight(context.Background(), ingressPreflightJob(), func(string) ([]byte, error) { return []byte(ingressSocketHeader), nil }, func(string) (string, error) { return "", errors.New("absent") })
	if err != nil || ev.PublicPortState != "not_observed" || !ev.SocketInventoryComplete || ev.ReadyToApply {
		t.Fatalf("empty inventory: %+v %v", ev, err)
	}
}

func TestPublicIngressPreflightValidationAndCancellation(t *testing.T) {
	for _, field := range []string{"public_port", "local_target"} {
		j := ingressPreflightJob()
		j.Params[field] = "invalid"
		_, err := collectPublicIngressPreflight(context.Background(), j, func(string) ([]byte, error) { t.Fatal("read before validation"); return nil, nil }, nil)
		if err == nil {
			t.Fatal("invalid job accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := collectPublicIngressPreflight(ctx, ingressPreflightJob(), func(string) ([]byte, error) { t.Fatal("read after cancellation"); return nil, nil }, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled inspection: %v", err)
	}
}

func TestPublicIngressPreflightSocketParser(t *testing.T) {
	sockets, err := parseIngressSocketTable([]byte(ingressSocketHeader+"0: 00000000000000000000000001000000:05A3 00000000000000000000000000000000:0000 0A\n"), true)
	if err != nil || len(sockets) != 1 || !sockets[0].ip.Equal(net.IPv6loopback) || sockets[0].port != 1443 {
		t.Fatalf("IPv6 parse: %+v %v", sockets, err)
	}
	for _, raw := range []string{"", "sl local_address", ingressSocketHeader + "bad\n", ingressSocketHeader + "0: 0100007F:0805 00000000:0000 XX\n", ingressSocketHeader + "0: BAD:0805 00000000:0000 0A\n", ingressSocketHeader + "0: 0100007F:FFFFF 00000000:0000 0A\n", ingressSocketHeader + "0: 0100007F:0000 00000000:0000 0A\n"} {
		if _, err := parseIngressSocketTable([]byte(raw), false); err == nil {
			t.Fatalf("accepted invalid proc data %q", raw)
		}
	}
}

func TestSignedPublicIngressPreflightObservesRealListenerWithoutMutation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux procfs socket inventory")
	}
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	publicLn, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer publicLn.Close()
	pub, key, err := release.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	j := ingressPreflightJob()
	j.Params["public_port"] = strconv.Itoa(publicLn.Addr().(*net.TCPAddr).Port)
	j.Params["local_target"] = ln.Addr().String()
	env, err := agentjob.Sign(key, j)
	if err != nil {
		t.Fatal(err)
	}
	var ack map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/agent/jobs":
			json.NewEncoder(w).Encode([]pulledJob{{ID: j.JobID, Signed: &env}})
		case "/api/agent/ack":
			json.NewDecoder(r.Body).Decode(&ack)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	sys := &fakeSystem{}
	a, err := New(Config{BCCURL: srv.URL, NodeID: j.NodeID, Token: "test-token", BCCJobKey: pub, StateDir: t.TempDir(), System: sys})
	if err != nil {
		t.Fatal(err)
	}
	results, err := a.RunOnce(context.Background())
	if err != nil || len(results) != 1 || results[0].Status != "succeeded" {
		t.Fatalf("signed dispatch: %+v %v ack=%+v", results, err, ack)
	}
	var ev publicIngressPreflightEvidence
	if err := json.Unmarshal([]byte(ack["Output"]), &ev); err != nil {
		t.Fatal(err)
	}
	if ev.PublicPortState != "occupied" || !ev.TargetListenerObserved || ev.ReadyToApply || ev.OwnershipVerified {
		t.Fatalf("live evidence: %+v", ev)
	}
	if len(sys.calls) != 0 {
		t.Fatalf("preflight ran host commands: %+v", sys.calls)
	}
	if err := ln.(*net.TCPListener).SetDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if conn, err := ln.Accept(); err == nil {
		conn.Close()
		t.Fatal("preflight connected to the existing listener")
	} else if timeout, ok := err.(net.Error); !ok || !timeout.Timeout() {
		t.Fatalf("existing listener stopped accepting: %v", err)
	}
	// The existing listener is still held; preflight must not close, steal,
	// replace, send payload to or reserve any endpoint.
	if other, err := net.Listen("tcp4", ln.Addr().String()); err == nil {
		other.Close()
		t.Fatal("existing listener was lost")
	}
	again, err := a.RunOnce(context.Background())
	if err != nil || len(again) != 1 || again[0].Status != "refused" {
		t.Fatalf("signed replay not refused: %+v %v", again, err)
	}
}
