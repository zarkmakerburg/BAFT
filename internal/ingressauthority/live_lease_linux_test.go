//go:build linux

package ingressauthority

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
	"github.com/zarkmakerburg/baft/internal/ingressinventory"
	"github.com/zarkmakerburg/baft/internal/release"
)

// The harness must independently mount empty disposable directories at both
// fixed paths. A root UID or an environment variable alone is insufficient.
func isolatedSignedLeaseLAB(t *testing.T) {
	t.Helper()
	if os.Getenv("BAFT_SIGNED_LEASE_LAB") != "1" {
		t.Skip("requires dedicated mount+network isolated Linux LAB")
	}
	if os.Geteuid() != 0 {
		t.Fatal("root private LAB required")
	}
	self, err := os.Readlink("/proc/self/ns/mnt")
	init, initErr := os.Readlink("/proc/1/ns/mnt")
	if err != nil || initErr != nil || self == init {
		t.Fatal("private mount namespace required")
	}
	mounts, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/etc/baft", "/etc/baft-agent", "/var/lib/baft-agent"} {
		found := false
		for _, line := range strings.Split(string(mounts), "\n") {
			fields := strings.Fields(line)
			if len(fields) > 5 && fields[4] == path {
				found = true
			}
		}
		marker, err := os.ReadFile(filepath.Join(path, ".signed-lease-lab"))
		if !found || err != nil || string(marker) != "disposable-signed-lease-lab" {
			t.Fatalf("missing private fixed-path mount %s", path)
		}
		local, err := os.Stat(path)
		host, hostErr := os.Stat("/proc/1/root" + path)
		if err != nil || hostErr == nil && os.SameFile(local, host) {
			t.Fatalf("host path exposed at %s", path)
		}
	}
}

func signedLeaseFixture(t *testing.T) (*PendingGuard, CommandV1, release.Envelope, *ReservationStore) {
	t.Helper()
	ctx := context.Background()
	base, err := os.MkdirTemp("/root", "baft-signed-lease-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	sources := filepath.Join(base, "sources")
	if err := os.Mkdir(sources, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("/etc/baft/ingress", 0700); err != nil {
		t.Fatal(err)
	}
	ns, err := os.Readlink("/proc/self/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	p := ingressinventory.InstalledManagedNodePolicyV1{SchemaVersion: 1, InventoryVersion: ingressinventory.ManagedCoverageVersion, NodeID: "ir-1", Platform: "linux", NetworkNamespace: ns, Transport: "dedicated", Manager: "systemd-socket-v1", SourceRoots: []string{sources}}
	raw, err := ingressinventory.MarshalInstalledManagedNodePolicyV1(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ingressinventory.InstalledPolicyPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(enrolledJobKeyPath, []byte(release.EncodePublic(public)), 0600); err != nil {
		t.Fatal(err)
	}
	b := bindingFixture(t)
	if err := os.Mkdir(InstalledAuthorityDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(InstalledAuthorityDirectory) })
	s, err := OpenInstalledManagedReservationStore(ctx, b.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ReservePending(b); err != nil {
		t.Fatal(err)
	}
	g, err := s.HoldPending(b)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = g.Close() })
	if _, err := g.BindInstalledPolicy(ctx); err != nil {
		t.Fatal(err)
	}
	if err := g.MarkApplying(ctx, b); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	c := CommandV1{IngressProtocol: ProtocolV1, JobID: "signed-live-apply", Action: commandIntentApply, IssuedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute), Binding: b}
	if err := g.RecordCommandIntent(ctx, c); err != nil {
		t.Fatal(err)
	}
	j, err := c.AgentJob()
	if err != nil {
		t.Fatal(err)
	}
	env, err := agentjob.Sign(private, j)
	if err != nil {
		t.Fatal(err)
	}
	return g, c, env, s
}

func TestManagedSignedLeaseLAB(t *testing.T) {
	isolatedSignedLeaseLAB(t)
	ctx := context.Background()
	t.Run("positive-command-close-and-restart-replay", func(t *testing.T) {
		g, c, env, s := signedLeaseFixture(t)
		l, err := g.MintSignedManagedLABLease(ctx, env)
		if err != nil {
			t.Fatal(err)
		}
		if err := l.CheckLockedIngress(ctx, c); err != nil {
			t.Fatal(err)
		}
		if _, err := json.Marshal(l); err == nil {
			t.Fatal("live lease serialized")
		}
		changed := c
		changed.JobID = "other-job"
		if err := l.CheckLockedIngress(ctx, changed); err == nil {
			t.Fatal("lease authorized another signed job")
		}
		changed = c
		changed.Action = commandIntentRetire
		if err := l.CheckLockedIngress(ctx, changed); err == nil {
			t.Fatal("Apply lease authorized Retire")
		}
		if _, err := g.MintSignedManagedLABLease(ctx, env); err == nil {
			t.Fatal("second mint authorized replay")
		}
		copyOfLease := *l
		if err := l.Close(); err != nil {
			t.Fatal(err)
		}
		if err := copyOfLease.CheckLockedIngress(ctx, c); err == nil {
			t.Fatal("copied lease survived close")
		}
		if err := g.Close(); err != nil {
			t.Fatal(err)
		}
		reopened, err := s.HoldPending(c.Binding)
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Close()
		if _, err := reopened.MintSignedManagedLABLease(ctx, env); err == nil {
			t.Fatal("reopened guard re-minted consumed command")
		}
	})
	t.Run("signature-and-wrong-key-refused", func(t *testing.T) {
		g, c, env, _ := signedLeaseFixture(t)
		forged := env
		forged.Payload = forged.Payload + "A"
		if _, err := g.MintSignedManagedLABLease(ctx, forged); err == nil {
			t.Fatal("tampered envelope minted")
		}
		_, wrong, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		j, err := c.AgentJob()
		if err != nil {
			t.Fatal(err)
		}
		wrongEnv, err := agentjob.Sign(wrong, j)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := g.MintSignedManagedLABLease(ctx, wrongEnv); err == nil {
			t.Fatal("untrusted signer minted")
		}
		l, err := g.MintSignedManagedLABLease(ctx, env)
		if err != nil {
			t.Fatalf("failed forgery consumed valid signed command: %v", err)
		}
		defer l.Close()
	})
	t.Run("alternate-store-cannot-bypass-global-claim", func(t *testing.T) {
		_, c, env, _ := signedLeaseFixture(t)
		other, err := OpenReservationStore(privateReservationDir(t))
		if err != nil {
			t.Fatal(err)
		}
		if err := other.ReservePending(c.Binding); err != nil {
			t.Fatal(err)
		}
		g, err := other.HoldPending(c.Binding)
		if err != nil {
			t.Fatal(err)
		}
		defer g.Close()
		if _, err := g.BindInstalledPolicy(ctx); err != nil {
			t.Fatal(err)
		}
		if err := g.MarkApplying(ctx, c.Binding); err != nil {
			t.Fatal(err)
		}
		if err := g.RecordCommandIntent(ctx, c); err != nil {
			t.Fatal(err)
		}
		if _, err := g.MintSignedManagedLABLease(ctx, env); err == nil {
			t.Fatal("alternate store minted competing numeric-port permit")
		}
	})
	t.Run("same-bytes-key-inode-replacement-revokes", func(t *testing.T) {
		g, c, env, _ := signedLeaseFixture(t)
		l, err := g.MintSignedManagedLABLease(ctx, env)
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()
		raw, err := os.ReadFile(enrolledJobKeyPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(enrolledJobKeyPath+".replacement", raw, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(enrolledJobKeyPath+".replacement", enrolledJobKeyPath); err != nil {
			t.Fatal(err)
		}
		if err := l.CheckLockedIngress(ctx, c); err == nil {
			t.Fatal("replaced trust inode kept permission")
		}
	})
	t.Run("source-arrival-revokes", func(t *testing.T) {
		g, c, env, _ := signedLeaseFixture(t)
		l, err := g.MintSignedManagedLABLease(ctx, env)
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()
		p, err := ingressinventory.LoadInstalledManagedNodePolicy(ctx, c.Binding.NodeID)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p.SourceRoots[0], "late.socket"), []byte("[Socket]\nListenStream=54619\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := l.CheckLockedIngress(ctx, c); err == nil {
			t.Fatal("late source retained permission")
		}
	})
	t.Run("guard-close-revokes", func(t *testing.T) {
		g, c, env, _ := signedLeaseFixture(t)
		l, err := g.MintSignedManagedLABLease(ctx, env)
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()
		if err := g.Close(); err != nil {
			t.Fatal(err)
		}
		if err := l.CheckLockedIngress(ctx, c); err == nil {
			t.Fatal("closed OS guard retained permission")
		}
	})
}

func TestManagedSignedLeaseEffectSerializationLAB(t *testing.T) {
	isolatedSignedLeaseLAB(t)
	ctx := context.Background()
	g, c, env, s := signedLeaseFixture(t)
	l, err := g.MintSignedManagedLABLease(ctx, env)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	started, finish := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- l.WithLockedIngress(ctx, c, func() error { close(started); <-finish; return nil }) }()
	<-started
	closing, closed := make(chan struct{}), make(chan error, 1)
	go func() { close(closing); closed <- g.Close() }()
	<-closing
	select {
	case err := <-closed:
		close(finish)
		t.Fatalf("OS guard closed during effect: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	other, err := s.HoldPending(c.Binding)
	if err == nil {
		other.Close()
		close(finish)
		t.Fatal("competing authority entered during effect")
	}
	close(finish)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	if err := l.WithLockedIngress(ctx, c, func() error { t.Error("effect ran after guard close"); return nil }); err == nil {
		t.Fatal("closed guard permitted effect")
	}
}

func TestLiveLeaseUnsignedConstructorInputsRefused(t *testing.T) {
	var g *PendingGuard
	if _, err := g.MintSignedManagedLABLease(context.Background(), release.Envelope{}); err == nil {
		t.Fatal("nil guard minted")
	}
	g = &PendingGuard{}
	if _, err := g.MintSignedManagedLABLease(context.Background(), release.Envelope{}); err == nil {
		t.Fatal("empty guard minted")
	}
	if _, err := g.MintSignedManagedLABLease(nil, release.Envelope{}); err == nil {
		t.Fatal("nil context minted")
	}
}
