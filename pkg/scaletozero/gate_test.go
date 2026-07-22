package scaletozero

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGateInFlightAccounting(t *testing.T) {
	reg := NewGateRegistry(nil)

	if reg.InFlight("f") != 0 {
		t.Fatalf("expected 0 in-flight initially")
	}
	t1 := reg.BeginInvocation("f")
	t2 := reg.BeginInvocation("f")
	if reg.InFlight("f") != 2 {
		t.Fatalf("expected 2 in-flight, got %d", reg.InFlight("f"))
	}
	t1.Release()
	if reg.InFlight("f") != 1 {
		t.Fatalf("expected 1 in-flight after one release, got %d", reg.InFlight("f"))
	}
	t1.Release() // idempotent second release must not underflow
	if reg.InFlight("f") != 1 {
		t.Fatalf("double release must be a no-op, got %d", reg.InFlight("f"))
	}
	t2.Release()
	if reg.InFlight("f") != 0 {
		t.Fatalf("expected 0 in-flight after all released, got %d", reg.InFlight("f"))
	}
}

// TestTryBeginReclaimRefusesInFlight is the load-bearing in-flight fence
// (SZ-01): a reclaim cannot start while an invocation is in flight.
func TestTryBeginReclaimRefusesInFlight(t *testing.T) {
	reg := NewGateRegistry(nil)
	tok := reg.BeginInvocation("f")

	if reg.TryBeginReclaim("f", reg.Generation("f")) {
		t.Fatalf("reclaim must be refused while an invocation is in flight")
	}

	tok.Release()
	if !reg.TryBeginReclaim("f", reg.Generation("f")) {
		t.Fatalf("reclaim must be permitted once no work is in flight")
	}
	reg.FinishReclaim("f", true)
}

// TestTryBeginReclaimRefusesStaleGeneration is the generation fence (SZ-08): a
// reclaim decision computed under an old generation cannot execute after a cold
// start bumped the generation.
func TestTryBeginReclaimRefusesStaleGeneration(t *testing.T) {
	reg := NewGateRegistry(nil)
	staleGen := reg.Generation("f") // 0

	// A cold start races in and bumps the generation.
	cs := reg.AcquireColdStart("f")
	if !cs.Leader {
		t.Fatalf("first cold start should be leader")
	}
	cs.Complete(nil)

	if reg.Generation("f") == staleGen {
		t.Fatalf("cold start must bump the generation")
	}
	if reg.TryBeginReclaim("f", staleGen) {
		t.Fatalf("reclaim under a stale generation must be refused")
	}
	// Under the live generation, with no work in flight, it is permitted.
	if !reg.TryBeginReclaim("f", reg.Generation("f")) {
		t.Fatalf("reclaim under the live generation should be permitted")
	}
	reg.FinishReclaim("f", true)
}

// TestConcurrentColdStartElectsSingleLeader is SZ-02: N concurrent cold starts
// produce exactly one leader; the rest wait and observe the leader's result.
// Deterministic: a channel barrier collects every acquisition BEFORE the
// leader completes, so no caller can slip into the post-ready window and the
// leader/follower split is fully sequenced without sleeps.
func TestConcurrentColdStartElectsSingleLeader(t *testing.T) {
	reg := NewGateRegistry(nil)

	const n = 25
	acquired := make(chan ColdStart, n)
	waitErrs := make(chan error, n)
	var doneWG sync.WaitGroup
	doneWG.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			defer doneWG.Done()
			cs := reg.AcquireColdStart("f")
			acquired <- cs
			if !cs.Leader {
				waitErrs <- cs.Wait(context.Background())
			}
		}()
	}

	// Barrier: every caller has acquired (leader or follower of the live op)
	// before the leader is allowed to complete.
	var leaders int64
	var leaderCS *ColdStart
	for i := 0; i < n; i++ {
		select {
		case cs := <-acquired:
			if cs.Leader {
				atomic.AddInt64(&leaders, 1)
				c := cs
				leaderCS = &c
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("caller %d never acquired", i)
		}
	}
	if got := atomic.LoadInt64(&leaders); got != 1 || leaderCS == nil {
		t.Fatalf("expected exactly 1 cold-start leader, got %d", got)
	}
	leaderCS.Complete(nil)

	for i := 0; i < n-1; i++ {
		select {
		case err := <-waitErrs:
			if err != nil {
				t.Fatalf("follower observed unexpected error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("follower %d never returned from Wait", i)
		}
	}
	waitTimeout(t, &doneWG, 5*time.Second)
}

// TestColdStartFollowerReceivesLeaderError: waiter fan-out of the leader's
// error. Deterministic: the follower signals its acquisition over a channel
// before the leader completes — no sleeps.
func TestColdStartFollowerReceivesLeaderError(t *testing.T) {
	reg := NewGateRegistry(nil)

	leader := reg.AcquireColdStart("f")
	if !leader.Leader {
		t.Fatalf("expected leader")
	}

	followerAcquired := make(chan bool, 1)
	got := make(chan error, 1)
	go func() {
		follower := reg.AcquireColdStart("f")
		followerAcquired <- follower.Leader
		if follower.Leader {
			got <- nil // unexpected: two leaders
			return
		}
		got <- follower.Wait(context.Background())
	}()

	select {
	case wasLeader := <-followerAcquired:
		if wasLeader {
			t.Fatal("second caller must not be a leader while the op is in flight")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("follower never acquired")
	}
	leader.Complete(context.DeadlineExceeded)

	select {
	case err := <-got:
		if err != context.DeadlineExceeded {
			t.Fatalf("follower should observe leader error, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("follower did not return")
	}
}

func TestMarkActivityMonotonic(t *testing.T) {
	reg := NewGateRegistry(nil)
	t1 := time.Now()
	reg.MarkActivity("f", t1)
	reg.MarkActivity("f", t1.Add(-time.Hour)) // older, must not move it back
	if !reg.LastActivity("f").Equal(t1) {
		t.Fatalf("MarkActivity must not move last activity backwards")
	}
}

func waitTimeout(t *testing.T, wg *sync.WaitGroup, d time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatal("timed out waiting for goroutines")
	}
}
