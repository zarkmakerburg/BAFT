//go:build linux

package operational

import (
	"testing"
	"time"

	"github.com/zarkmakerburg/baft/internal/agentjob"
)

func waitPortFree(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !portFree(port) {
		if time.Now().After(deadline) {
			t.Fatalf("port %d never closed", port)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// A signed job is single use. The identical envelope, replayed by anyone
// (here: the transport in front of BCC), is refused by the live Agent and by
// a restarted Agent, and never re-binds, re-installs or re-reports anything.
func TestOperationalIngressV1Gate_RefusesReplayedSignedJob(t *testing.T) {
	f := setupFullPath(t, 54751, "127.0.0.1:1443")
	f.legit(t)
	body := f.tap.lastCaptured()
	if len(body) == 0 {
		t.Fatal("the honest signed job was not observed on the wire")
	}
	before := f.snap(t)
	replay := func(a *agentRig) {
		t.Helper()
		f.tap.setHook(func([]byte) []byte { return body })
		defer f.tap.setHook(nil)
		res := a.runOnce()
		if len(res) != 1 || res[0].Status != "refused" {
			t.Fatalf("replayed envelope was not refused: %+v", res)
		}
	}
	replay(f.a)
	if after := f.snap(t); after != before {
		t.Fatalf("replay changed state:\nbefore=%s\nafter=%s", before, after)
	}
	f.mustStillServe(t)
	// BCC does not re-issue it either: enqueue is an idempotent no-op that
	// creates no second job and cannot reset the consumed dispatch.
	jobs := len(f.b.store.ListJobs())
	if err := f.b.store.EnqueueOperationalIngress(f.tn.ID); err != nil {
		t.Fatalf("idempotent re-enqueue: %v", err)
	}
	if len(f.b.store.ListJobs()) != jobs {
		t.Fatal("re-enqueue created another job")
	}
	// Restart: the seen-job ledger and reservation survive the process.
	dir := f.a.dir
	f.a.kill()
	waitPortFree(t, f.port)
	a2 := startAgent(t, f.b, f.h, dir, &agentOpt{url: f.tap.srv.URL, client: f.b.cl})
	replay(a2)
	if !portFree(f.port) {
		t.Fatal("a replay after restart re-bound the listener")
	}
	if n := f.h.baftRules(); n != 1 {
		t.Fatalf("rule count after replay on restart: %d", n)
	}
	op := f.b.op(f.tn.ID)
	if op.Receipt == nil || op.Receipt.Phase != "applied" {
		t.Fatalf("receipt changed by replay: %+v", op.Receipt)
	}
}

// Expiry is enforced at every hop: BCC never queues or signs a command whose
// window passed (and records the compensation), and an Agent refuses a
// correctly signed but expired or not-yet-valid job.
func TestOperationalIngressV1Gate_RefusesExpiredSignedJob(t *testing.T) {
	f := setupFullPath(t, 54752, "127.0.0.1:1443")
	f.window = 2 * time.Second
	f.stage(t)
	f.a.openGate()
	time.Sleep(4 * time.Second)
	if err := f.b.store.EnqueueOperationalIngress(f.tn.ID); err == nil {
		t.Fatal("BCC queued a command outside its signed window")
	}
	if op := f.b.op(f.tn.ID); op.Dispatch == nil || op.Dispatch.Phase != "expired" {
		t.Fatalf("expired dispatch not compensated durably: %+v", op.Dispatch)
	}
	if res := f.a.runOnce(); len(res) != 0 {
		t.Fatalf("expired command reached the Agent: %+v", res)
	}
	// A late delivery of a validly signed but expired envelope.
	expired := forgedJob(f.cmd, nil)
	res := f.deliver(t, expired)
	if len(res) != 1 || res[0].Status != "refused" {
		t.Fatalf("expired signed job was not refused: %+v", res)
	}
	f.noEffects(t)
	// ...and one that is not valid yet.
	future := forgedJob(f.cmd, func(j *agentjob.Job) {
		j.JobID = "future-job"
		j.IssuedAt = time.Now().Add(time.Hour)
		j.ExpiresAt = time.Now().Add(2 * time.Hour)
	})
	res = f.deliver(t, future)
	if len(res) != 1 || res[0].Status != "refused" {
		t.Fatalf("not-yet-valid signed job was not refused: %+v", res)
	}
	f.noEffects(t)
	// An absurdly long window is rejected as well.
	long := forgedJob(f.cmd, func(j *agentjob.Job) {
		j.JobID = "long-job"
		j.IssuedAt = time.Now().Add(-time.Second)
		j.ExpiresAt = time.Now().Add(400 * 24 * time.Hour)
	})
	if res = f.deliver(t, long); len(res) != 1 || res[0].Status == "succeeded" {
		t.Fatalf("over-long window applied: %+v", res)
	}
	f.noEffects(t)
}
