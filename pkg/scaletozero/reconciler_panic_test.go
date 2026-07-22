package scaletozero

import (
	"context"
	"testing"
	"time"
)

// panicController panics on ReclaimToZero for a chosen function, to prove the
// reconciler contains the panic and still releases the reclaim lock.
type panicController struct {
	*fakeController
	panicOn string
}

func (p *panicController) ReclaimToZero(ctx context.Context, fn string) (ReclaimReport, error) {
	if fn == p.panicOn {
		panic("simulated Docker SDK explosion")
	}
	return p.fakeController.ReclaimToZero(ctx, fn)
}

// TestReconcilePanicIsContainedAndGateReleased proves the RT-2xx panic-safety
// fix: a panic inside ReclaimToZero (a raw Docker SDK call) must NOT propagate
// out of ReconcileOnce (which would crash the whole gateway), and FinishReclaim
// must still run so the gate's scale lock is released — otherwise that function
// could never cold-start or be reclaimed again (permanently wedged).
func TestReconcilePanicIsContainedAndGateReleased(t *testing.T) {
	base := newFakeController()
	base.observed["boom"] = 1
	ctrl := &panicController{fakeController: base, panicOn: "boom"}

	gates := NewGateRegistry(func() time.Time { return t0 })
	gates.MarkActivity("boom", t0) // idle long enough to trigger reclaim
	leases := NewLeaseRegistry(func() time.Time { return t1 })

	rec := newTestReconciler(ctrl, fakePolicies{[]DeclaredFunction{{Name: "boom", Policy: enabledPolicy()}}}, gates, leases, newRecordingMetrics())

	// Must not panic. safeReclaim contains the panic at the per-function level
	// and the pass CONTINUES (does not crash the process, does not abort the
	// whole pass) — so ReconcileOnce completes without error and simply did not
	// reclaim the panicking function.
	if _, err := rec.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("a contained per-function reclaim panic must not fail the pass, got %v", err)
	}
	if ctrl.reclaimedCount("boom") != 0 {
		t.Fatalf("panicking reclaim must not count as reclaimed, got %d", ctrl.reclaimedCount("boom"))
	}

	// The gate lock must be released: a fresh reclaim attempt under the live
	// generation with no in-flight work must be permitted. If FinishReclaim had
	// been skipped, scaleDone would still be set and this would return false.
	if !gates.TryBeginReclaim("boom", gates.Generation("boom")) {
		t.Fatal("gate is wedged: FinishReclaim did not run after the panic")
	}
	gates.FinishReclaim("boom", false)
}

// TestReconcilePanicDoesNotStopOtherFunctions: a panic reclaiming one function
// must not prevent the pass from being retryable; a subsequent pass with the
// panic removed reclaims normally.
func TestReconcilePanicPassIsRetryable(t *testing.T) {
	base := newFakeController()
	base.observed["boom"] = 1
	ctrl := &panicController{fakeController: base, panicOn: "boom"}

	gates := NewGateRegistry(func() time.Time { return t0 })
	gates.MarkActivity("boom", t0)
	leases := NewLeaseRegistry(func() time.Time { return t1 })

	rec := newTestReconciler(ctrl, fakePolicies{[]DeclaredFunction{{Name: "boom", Policy: enabledPolicy()}}}, gates, leases, newRecordingMetrics())

	// First pass: panic is contained, function not reclaimed, gate released.
	if _, err := rec.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("first pass must not fail on a contained panic, got %v", err)
	}
	if ctrl.reclaimedCount("boom") != 0 {
		t.Fatalf("panicking function must not be reclaimed, got %d", ctrl.reclaimedCount("boom"))
	}

	// Remove the panic; the next pass must reclaim cleanly (gate was released).
	ctrl.panicOn = ""
	if _, err := rec.ReconcileOnce(context.Background()); err != nil {
		t.Fatalf("retry pass should succeed, got %v", err)
	}
	if ctrl.reclaimedCount("boom") != 1 {
		t.Fatalf("expected exactly 1 successful reclaim on retry, got %d", ctrl.reclaimedCount("boom"))
	}
}
