package scaletozero

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// All tests in this file are deterministic: every ordering is enforced by
// channel handshakes or strict sequencing, never by sleeps (RT-202/RT-213).

// TestAcquireColdStartCtxCancelledDuringReclaim: a caller parked behind an
// in-progress reclaim must honor context cancellation instead of blocking
// until the reclaim finishes (RT-213 cancellation).
func TestAcquireColdStartCtxCancelledDuringReclaim(t *testing.T) {
	reg := NewGateRegistry(nil)

	if !reg.TryBeginReclaim("f", reg.Generation("f")) {
		t.Fatalf("reclaim must start on an idle gate")
	}

	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan error, 1)
	go func() {
		_, err := reg.AcquireColdStartCtx(ctx, "f")
		got <- err
	}()

	// Whether the goroutine has parked on the reclaim channel yet or not, the
	// cancel wakes it either way (select on ctx.Done vs scaleDone).
	cancel()

	select {
	case err := <-got:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled while parked behind reclaim, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("acquire did not honor cancellation (deadlock behind reclaim)")
	}

	// The reclaim is still exclusively owned and finishes normally.
	reg.FinishReclaim("f", false)
}

// TestReportZeroObservedDemotesOnlyCurrentGeneration: the crash-recovery
// demotion is generation-fenced — a stale observation can never demote
// readiness established by a newer cold start (RT-213 stale-generation
// rejection).
func TestReportZeroObservedDemotesOnlyCurrentGeneration(t *testing.T) {
	reg := NewGateRegistry(nil)

	cs := reg.AcquireColdStart("f")
	if !cs.Leader {
		t.Fatalf("expected leader on first acquire")
	}
	cs.Complete(nil)
	gen := reg.Generation("f")
	if !reg.Ready("f") {
		t.Fatalf("gate must be ready after successful cold start")
	}

	// A stale-generation observation must be rejected.
	if reg.ReportZeroObserved("f", gen-1) {
		t.Fatalf("stale-generation demotion must be rejected")
	}
	if !reg.Ready("f") {
		t.Fatalf("stale-generation demotion must not change readiness")
	}

	// A current-generation observation demotes readiness exactly once.
	if !reg.ReportZeroObserved("f", gen) {
		t.Fatalf("current-generation demotion must be accepted")
	}
	if reg.Ready("f") {
		t.Fatalf("gate must not be ready after demotion")
	}
	if reg.ReportZeroObserved("f", gen) {
		t.Fatalf("second demotion must be a no-op (already demoted)")
	}

	// A fresh caller must now elect a NEW leader with a bumped generation.
	next := reg.AcquireColdStart("f")
	if !next.Leader {
		t.Fatalf("after crash demotion a fresh leader must be electable")
	}
	if got := reg.Generation("f"); got != gen+1 {
		t.Fatalf("crash re-election must advance the generation %d -> %d, got %d", gen, gen+1, got)
	}
	next.Complete(nil)
}

// TestReportZeroObservedRejectedDuringScaleOps: demotion is refused while any
// scale operation (cold start or reclaim) is in progress, so the state machine
// never loses an in-flight transition.
func TestReportZeroObservedRejectedDuringScaleOps(t *testing.T) {
	reg := NewGateRegistry(nil)

	// During a cold start.
	cs := reg.AcquireColdStart("f")
	if !cs.Leader {
		t.Fatalf("expected leader")
	}
	if reg.ReportZeroObserved("f", reg.Generation("f")) {
		t.Fatalf("demotion must be refused during a cold start")
	}
	cs.Complete(nil)

	// During a reclaim.
	if !reg.TryBeginReclaim("f", reg.Generation("f")) {
		t.Fatalf("reclaim must start")
	}
	if reg.ReportZeroObserved("f", reg.Generation("f")) {
		t.Fatalf("demotion must be refused during a reclaim")
	}
	reg.FinishReclaim("f", true)

	// After a committed reclaim the gate is already not-ready; demotion is a
	// no-op rather than an error.
	if reg.ReportZeroObserved("f", reg.Generation("f")) {
		t.Fatalf("demotion of a not-ready gate must be a no-op")
	}
}

// TestFailedStartFollowerFanOutAndReElection: a follower receives the leader's
// error (waiter fan-out), and because a failed start returns the gate to zero,
// the SAME follower can then elect itself the fresh leader (failed-start
// retry). Sequencing is enforced by channel handshakes.
func TestFailedStartFollowerFanOutAndReElection(t *testing.T) {
	reg := NewGateRegistry(nil)
	errBoom := errors.New("scale-from-zero exploded")

	leader := reg.AcquireColdStart("f")
	if !leader.Leader {
		t.Fatalf("expected leader")
	}
	genLeader1 := reg.Generation("f")

	followerAcquired := make(chan struct{})
	followerResult := make(chan error, 1)
	reElected := make(chan bool, 1)
	go func() {
		cs := reg.AcquireColdStart("f")
		if cs.Leader {
			followerResult <- errors.New("second caller must not be leader while op in flight")
			close(followerAcquired)
			return
		}
		close(followerAcquired)
		err := cs.Wait(context.Background())
		followerResult <- err

		// Failed-start retry: the gate fell back to zero, so re-acquiring must
		// elect this caller as the fresh leader.
		retry := reg.AcquireColdStart("f")
		reElected <- retry.Leader
		if retry.Leader {
			retry.Complete(nil)
		}
	}()

	// The follower is guaranteed to have acquired BEFORE the leader completes.
	<-followerAcquired
	leader.Complete(errBoom)

	select {
	case err := <-followerResult:
		if !errors.Is(err, errBoom) {
			t.Fatalf("follower must observe the leader's error, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("follower did not receive leader result")
	}

	select {
	case wasLeader := <-reElected:
		if !wasLeader {
			t.Fatalf("after a failed start the retrying follower must become the fresh leader")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("retry did not complete")
	}

	if got := reg.Generation("f"); got != genLeader1+1 {
		t.Fatalf("re-election must advance the generation exactly once more: %d -> %d", genLeader1, got)
	}
	if !reg.Ready("f") {
		t.Fatalf("gate must be ready after the retry leader succeeded")
	}
}

// TestHundredDeterministicColdStartCycles runs 100 full cold-start cycles, each
// with 8 concurrent acquirers. Per cycle: exactly ONE leader, every follower
// observes success, and the generation advances exactly once. The leader only
// completes after ALL acquirers have registered (channel barrier), so no
// acquirer can slip into the post-ready window — every interleaving inside a
// cycle yields the same observable outcome.
func TestHundredDeterministicColdStartCycles(t *testing.T) {
	const cycles = 100
	const callers = 8

	reg := NewGateRegistry(nil)

	for cycle := 0; cycle < cycles; cycle++ {
		type outcome struct {
			leader bool
			err    error
		}
		genBefore := reg.Generation("f")

		acquired := make(chan ColdStart, callers)
		results := make(chan outcome, callers)
		var wg sync.WaitGroup
		for i := 0; i < callers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				cs, err := reg.AcquireColdStartCtx(context.Background(), "f")
				if err != nil {
					results <- outcome{err: err}
					return
				}
				acquired <- cs
				if cs.Leader {
					// Leader parks until the barrier lets it complete; the
					// completion itself happens on the main goroutine.
					results <- outcome{leader: true}
					return
				}
				results <- outcome{err: cs.Wait(context.Background())}
			}()
		}

		// Barrier: collect ALL acquisitions, find the single leader, then let
		// it complete. Followers are guaranteed to be waiting on the live op.
		var leaderCS *ColdStart
		leaders := 0
		for i := 0; i < callers; i++ {
			select {
			case cs := <-acquired:
				if cs.Leader {
					leaders++
					c := cs
					leaderCS = &c
				}
			case <-time.After(10 * time.Second):
				t.Fatalf("cycle %d: acquirer %d never registered", cycle, i)
			}
		}
		if leaders != 1 || leaderCS == nil {
			t.Fatalf("cycle %d: expected exactly 1 leader, got %d", cycle, leaders)
		}
		leaderCS.Complete(nil)

		for i := 0; i < callers; i++ {
			select {
			case res := <-results:
				if res.err != nil {
					t.Fatalf("cycle %d: caller failed: %v", cycle, res.err)
				}
			case <-time.After(10 * time.Second):
				t.Fatalf("cycle %d: caller %d never finished", cycle, i)
			}
		}
		wg.Wait()

		if got := reg.Generation("f"); got != genBefore+1 {
			t.Fatalf("cycle %d: generation must advance exactly once (%d -> %d)", cycle, genBefore, got)
		}
		if !reg.Ready("f") {
			t.Fatalf("cycle %d: gate must be ready after completion", cycle)
		}

		// Reset to zero for the next cycle through the reclaim path, which is
		// itself part of the state machine under test.
		if !reg.TryBeginReclaim("f", reg.Generation("f")) {
			t.Fatalf("cycle %d: reclaim must be permitted on the idle ready gate", cycle)
		}
		reg.FinishReclaim("f", true)
		if reg.Ready("f") {
			t.Fatalf("cycle %d: gate must not be ready after reclaim", cycle)
		}
	}
}
