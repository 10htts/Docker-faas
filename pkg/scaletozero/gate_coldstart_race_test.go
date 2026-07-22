package scaletozero

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

// TestLateColdStartAfterReadyDoesNotReElect is the CV-07 fix: once a cold start
// has succeeded (the replica is ready) a LATE caller must NOT elect a second
// leader. It must observe readiness and proceed against the ready replica.
// Electing a second leader here would mean a redundant scale-from-zero (and a
// stray container) for a function that is already up.
//
// This test is DETERMINISTIC (no timing / no sleeps): it is the non-vacuity
// discriminator for the fix. Against the pre-fix gate (which ignored `ready`)
// the late caller returns Leader=true and the generation fence advances a second
// time, so this test fails hard.
func TestLateColdStartAfterReadyDoesNotReElect(t *testing.T) {
	reg := NewGateRegistry(nil)

	first := reg.AcquireColdStart("f")
	if !first.Leader {
		t.Fatalf("first caller must be the cold-start leader")
	}
	genAfterFirst := reg.Generation("f")
	first.Complete(nil) // cold start succeeded; replica is ready.

	// A late caller arriving AFTER readiness.
	late := reg.AcquireColdStart("f")
	if late.Leader {
		t.Fatalf("late caller after readiness must NOT become a second leader")
	}
	if err := late.Wait(context.Background()); err != nil {
		t.Fatalf("late caller Wait must return nil (replica ready), got %v", err)
	}
	if got := reg.Generation("f"); got != genAfterFirst {
		t.Fatalf("late caller must not trigger a second cold start: generation moved %d -> %d",
			genAfterFirst, got)
	}

	// Completeness / anti-hardcode guard: after a GENUINE reclaim (ready=false),
	// a subsequent caller MUST be able to elect a fresh cold-start leader again.
	if !reg.TryBeginReclaim("f", reg.Generation("f")) {
		t.Fatalf("reclaim under the live generation with no in-flight work must be permitted")
	}
	reg.FinishReclaim("f", true) // actually scaled to zero -> ready=false

	after := reg.AcquireColdStart("f")
	if !after.Leader {
		t.Fatalf("after a genuine reclaim, a new caller MUST elect a fresh cold-start leader")
	}
	if got := reg.Generation("f"); got <= genAfterFirst {
		t.Fatalf("a fresh cold start after reclaim must advance the generation past %d, got %d",
			genAfterFirst, got)
	}
	after.Complete(nil)
}

// TestColdStartLateArrivalsNeverReElect stresses the late-arrival-after-ready
// window repeatedly: for each iteration one leader cold-starts and completes,
// then a wave of concurrent late callers arrives straddling the ready
// transition. Not one of them may elect an extra leader. Looped well past the
// 50-repeat bar the revalidation asked for. Against the pre-fix gate the late
// callers that arrive after Complete elect extra leaders and this fails.
func TestColdStartLateArrivalsNeverReElect(t *testing.T) {
	const iterations = 200
	const lateCallers = 16

	for iter := 0; iter < iterations; iter++ {
		reg := NewGateRegistry(nil)

		leader := reg.AcquireColdStart("f")
		if !leader.Leader {
			t.Fatalf("iter %d: first caller must be leader", iter)
		}

		var extraLeaders int64
		var wg sync.WaitGroup
		wg.Add(lateCallers)
		release := make(chan struct{})

		for i := 0; i < lateCallers; i++ {
			go func() {
				defer wg.Done()
				<-release
				cs := reg.AcquireColdStart("f")
				if cs.Leader {
					atomic.AddInt64(&extraLeaders, 1)
					// A stray leader must still complete so no follower hangs.
					cs.Complete(nil)
					return
				}
				_ = cs.Wait(context.Background())
			}()
		}

		// Drive the ready transition, then release the late wave so callers
		// straddle it: some observe cold-in-progress, some observe ready.
		leader.Complete(nil)
		close(release)
		wg.Wait()

		if extra := atomic.LoadInt64(&extraLeaders); extra != 0 {
			t.Fatalf("iter %d: late arrivals after readiness elected %d extra leader(s)", iter, extra)
		}
	}
}
