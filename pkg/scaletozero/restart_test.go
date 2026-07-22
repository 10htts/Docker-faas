package scaletozero

// Restart-safety (RT-203) and storm-determinism (RT-211) tests.
//
// Gates and leases are IN-MEMORY: a gateway restart loses all in-flight
// accounting, activity history, generations, and durable-work leases, while the
// provider's containers keep running. These tests prove the reconciler's two
// restart properties:
//
//   - it NEVER reclaims a surviving function before one FULL idle window has
//     elapsed after the restart (the control plane, whose default lease TTL is
//     30s and which re-posts leases continuously, always gets the chance to
//     re-assert durable work first); and
//   - it still CONVERGES: a genuinely idle function is reclaimed exactly one
//     idle window after the first post-restart observation.
//
// Every test drives an injected fake clock and channel-sequenced goroutines;
// there are no sleeps and no free-running loops, so each interleaving is
// explicitly scripted and the tests are deterministic.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/docker-faas/docker-faas/pkg/faascontract"
)

// --- deterministic fixtures (rst-prefixed to keep this file's symbols
// disjoint from the other test files in the package) ---

type rstClock struct {
	mu sync.Mutex
	t  time.Time
}

func newRstClock(start time.Time) *rstClock { return &rstClock{t: start} }

func (c *rstClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *rstClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// rstController is a fake ReplicaController that counts every ReclaimToZero
// ATTEMPT (including failed ones) and exposes hooks fired during observation and
// reclaim so tests can inject invocations at exact points of a reconcile pass.
type rstController struct {
	mu         sync.Mutex
	observed   map[string]int
	fns        []string
	reclaimErr map[string]error
	attempts   map[string]int
	succeeded  map[string]int
	warmCalls  map[string]int
	onObserve  func(fn string)
	onReclaim  func(fn string)
}

func newRstController() *rstController {
	return &rstController{
		observed:   map[string]int{},
		reclaimErr: map[string]error{},
		attempts:   map[string]int{},
		succeeded:  map[string]int{},
		warmCalls:  map[string]int{},
	}
}

func (c *rstController) setObserved(fn string, n int) {
	c.mu.Lock()
	c.observed[fn] = n
	c.mu.Unlock()
}

func (c *rstController) setReclaimErr(fn string, err error) {
	c.mu.Lock()
	if err == nil {
		delete(c.reclaimErr, fn)
	} else {
		c.reclaimErr[fn] = err
	}
	c.mu.Unlock()
}

func (c *rstController) attemptCount(fn string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.attempts[fn]
}

func (c *rstController) successCount(fn string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.succeeded[fn]
}

func (c *rstController) observedCount(fn string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.observed[fn]
}

func (c *rstController) ObservedReplicas(_ context.Context, fn string) (int, error) {
	c.mu.Lock()
	n := c.observed[fn]
	hook := c.onObserve
	c.mu.Unlock()
	if hook != nil {
		hook(fn)
	}
	return n, nil
}

func (c *rstController) ObservedFunctions(context.Context) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.fns...), nil
}

func (c *rstController) ReclaimToZero(_ context.Context, fn string) (ReclaimReport, error) {
	c.mu.Lock()
	c.attempts[fn]++
	err := c.reclaimErr[fn]
	hook := c.onReclaim
	c.mu.Unlock()
	if hook != nil {
		hook(fn)
	}
	if err != nil {
		return ReclaimReport{}, err
	}
	c.mu.Lock()
	c.observed[fn] = 0
	c.succeeded[fn]++
	c.mu.Unlock()
	return ReclaimReport{ContainersRemoved: 1}, nil
}

func (c *rstController) EnsureWarmMinimum(_ context.Context, fn string, min int) error {
	c.mu.Lock()
	c.warmCalls[fn] = min
	c.observed[fn] = min
	c.mu.Unlock()
	return nil
}

// stormMetrics is a MetricsSink whose Decision callback runs between the
// reconciler's Decide and its TryBeginReclaim CAS — the exact snapshot-vs-CAS
// window RT-211 is about.
type stormMetrics struct {
	NopMetrics
	onDecision func(fn string, action Action)
}

func (m *stormMetrics) Decision(fn string, action Action) {
	if m.onDecision != nil {
		m.onDecision(fn, action)
	}
}

func rstPolicy() Policy {
	return Policy{Enabled: true, IdleDuration: 60 * time.Second}
}

func rstLease(fn string, gen uint64, running, ttlSeconds int) faascontract.ActivityLeaseRequest {
	return faascontract.ActivityLeaseRequest{
		ContractVersion: faascontract.ContractVersion,
		Function:        fn,
		Generation:      gen,
		Running:         running,
		LeaseTTLSeconds: ttlSeconds,
	}
}

func newRstReconciler(clk *rstClock, ctrl ReplicaController, fns []DeclaredFunction, gates *GateRegistry, leases *LeaseRegistry, m MetricsSink) *IdleReconciler {
	return NewIdleReconciler(ReconcilerConfig{
		Controller: ctrl,
		Policies:   fakePolicies{fns},
		Gates:      gates,
		Leases:     leases,
		Metrics:    m,
		Logger:     testLogger(),
		Clock:      clk.Now,
	})
}

var rstStart = time.Date(2026, 7, 21, 8, 0, 0, 0, time.UTC)

// TestRestartWithDurableWorkLeaseRenewalPreventsReclaim (RT-203a): after a
// restart wipes gates and leases, a surviving replica with durable work is
// protected as long as the control plane keeps renewing its lease — across many
// idle windows — and once renewals stop, reclaim happens only after one FULL
// idle window since the last renewal, never at lease expiry itself.
func TestRestartWithDurableWorkLeaseRenewalPreventsReclaim(t *testing.T) {
	const fn = "worker"
	ctx := context.Background()
	clk := newRstClock(rstStart)
	ctrl := newRstController()
	ctrl.setObserved(fn, 1)

	// Pre-restart process: real invocation history and a durable lease existed.
	oldGates := NewGateRegistry(clk.Now)
	oldLeases := NewLeaseRegistry(clk.Now)
	tok := oldGates.BeginInvocation(fn)
	tok.Release()
	oldLeases.Apply(rstLease(fn, 0, 1, 30))

	// RESTART: every bit of that in-memory state is gone. Fresh registries and
	// a fresh reconciler over the SAME provider-observed container state.
	gates := NewGateRegistry(clk.Now)
	leases := NewLeaseRegistry(clk.Now)
	m := newRecordingMetrics()
	rec := newRstReconciler(clk, ctrl, []DeclaredFunction{{Name: fn, Policy: rstPolicy()}}, gates, leases, m)

	// The control plane re-posts its durable lease before the idle window can
	// elapse (default TTL 30s, renewed every 20s here) under the live (fresh)
	// generation.
	leases.Apply(rstLease(fn, gates.Generation(fn), 1, 30))

	// 8 renewal cycles x 20s = 160s total, far beyond the 60s idle window: the
	// function must never be reclaimed while the lease keeps being renewed.
	for i := 0; i < 8; i++ {
		res, err := rec.ReconcileOnce(ctx)
		if err != nil {
			t.Fatalf("renewal cycle %d: reconcile: %v", i, err)
		}
		if res.Reclaimed != 0 || ctrl.attemptCount(fn) != 0 {
			t.Fatalf("renewal cycle %d: renewed durable lease must prevent reclaim, got %+v (attempts=%d)", i, res, ctrl.attemptCount(fn))
		}
		if m.decisions[fn] != ActionHold {
			t.Fatalf("renewal cycle %d: decision = %v, want Hold", i, m.decisions[fn])
		}
		clk.Advance(20 * time.Second)
		leases.Apply(rstLease(fn, gates.Generation(fn), 1, 30))
	}

	// Renewals stop: the durable job finished. The lease expires 30s after the
	// last renewal, but reclaim must wait for a FULL idle window measured from
	// the last durable activity — not fire at expiry.
	clk.Advance(30 * time.Second) // exactly at lease expiry; 30s since last renewal
	res, err := rec.ReconcileOnce(ctx)
	if err != nil {
		t.Fatalf("reconcile at lease expiry: %v", err)
	}
	if res.Reclaimed != 0 || ctrl.attemptCount(fn) != 0 {
		t.Fatalf("reclaim fired at lease expiry; it must wait a full idle window after the last durable activity: %+v", res)
	}

	clk.Advance(29 * time.Second) // 59s since last renewal: one second short
	res, err = rec.ReconcileOnce(ctx)
	if err != nil {
		t.Fatalf("reconcile inside idle window: %v", err)
	}
	if res.Reclaimed != 0 || ctrl.attemptCount(fn) != 0 {
		t.Fatalf("reclaim fired before the idle window elapsed: %+v", res)
	}

	clk.Advance(1 * time.Second) // exactly one idle window since last renewal
	res, err = rec.ReconcileOnce(ctx)
	if err != nil {
		t.Fatalf("reconcile after idle window: %v", err)
	}
	if res.Reclaimed != 1 || ctrl.attemptCount(fn) != 1 || ctrl.successCount(fn) != 1 {
		t.Fatalf("expected exactly one reclaim once the idle window elapsed with no renewal, got %+v (attempts=%d)", res, ctrl.attemptCount(fn))
	}
	if ctrl.observedCount(fn) != 0 {
		t.Fatalf("function must converge to zero observed replicas")
	}
}

// TestRestartIdleConvergenceSeedsWindowThenReclaims (RT-203b): fresh registries
// with a surviving replica and no activity of any kind. The first pass must
// seed the idle clock and HOLD — even when the wall clock is arbitrarily far
// ahead — and reclaim must happen exactly one idle window after seeding, with
// exactly one ReclaimToZero call, leaving the gate reusable.
func TestRestartIdleConvergenceSeedsWindowThenReclaims(t *testing.T) {
	const fn = "idlefn"
	ctx := context.Background()
	// A wall clock far in the future: seeding must not depend on any relation
	// between process start time and container age.
	clk := newRstClock(time.Date(2031, 3, 9, 23, 45, 0, 0, time.UTC))
	ctrl := newRstController()
	ctrl.setObserved(fn, 1)

	gates := NewGateRegistry(clk.Now)
	leases := NewLeaseRegistry(clk.Now)
	m := newRecordingMetrics()
	rec := newRstReconciler(clk, ctrl, []DeclaredFunction{{Name: fn, Policy: rstPolicy()}}, gates, leases, m)

	// Pass 1: no recorded activity anywhere; the pass must seed and HOLD.
	res, err := rec.ReconcileOnce(ctx)
	if err != nil {
		t.Fatalf("seeding pass: %v", err)
	}
	if res.Held != 1 || res.Reclaimed != 0 || ctrl.attemptCount(fn) != 0 {
		t.Fatalf("seeding pass must hold: %+v (attempts=%d)", res, ctrl.attemptCount(fn))
	}
	if gates.LastActivity(fn).IsZero() {
		t.Fatalf("seeding pass must record the first observation as activity")
	}

	// One second short of the idle window since seeding: still held.
	clk.Advance(rstPolicy().IdleDuration - time.Second)
	res, err = rec.ReconcileOnce(ctx)
	if err != nil {
		t.Fatalf("pre-window pass: %v", err)
	}
	if res.Reclaimed != 0 || ctrl.attemptCount(fn) != 0 {
		t.Fatalf("reclaimed before a full idle window elapsed post-restart: %+v", res)
	}

	// Exactly one idle window after seeding: converge to zero.
	clk.Advance(time.Second)
	res, err = rec.ReconcileOnce(ctx)
	if err != nil {
		t.Fatalf("convergence pass: %v", err)
	}
	if res.Reclaimed != 1 || ctrl.attemptCount(fn) != 1 || ctrl.successCount(fn) != 1 {
		t.Fatalf("expected exactly one reclaim at the idle window boundary, got %+v (attempts=%d)", res, ctrl.attemptCount(fn))
	}
	if ctrl.observedCount(fn) != 0 {
		t.Fatalf("function must be at zero observed replicas")
	}

	// The reclaim cycle must leave the gate reusable: generation unchanged
	// (only a cold start bumps it) and the reclaim lock fully released.
	if gen := gates.Generation(fn); gen != 0 {
		t.Fatalf("reclaim must not bump the generation, got %d", gen)
	}
	if !gates.TryBeginReclaim(fn, gates.Generation(fn)) {
		t.Fatalf("gate must be reusable after FinishReclaim")
	}
	gates.FinishReclaim(fn, false)

	// A further pass converges to AlreadyZero with no second ReclaimToZero.
	res, err = rec.ReconcileOnce(ctx)
	if err != nil {
		t.Fatalf("post-convergence pass: %v", err)
	}
	if res.AlreadyZero != 1 || ctrl.attemptCount(fn) != 1 {
		t.Fatalf("post-convergence pass must be a no-op (AlreadyZero), got %+v (attempts=%d)", res, ctrl.attemptCount(fn))
	}
}

// TestRestartMidReclaimConvergesIdempotently (RT-203c): the pre-restart process
// died in the middle of a reclaim — one of two containers already removed, so
// the provider observes a single running replica and the fresh process has no
// gate state. A later pass (after the seeded window) must finish the job with a
// second, idempotent ReclaimToZero and converge to zero without errors.
func TestRestartMidReclaimConvergesIdempotently(t *testing.T) {
	const fn = "draining"
	ctx := context.Background()
	clk := newRstClock(rstStart)
	ctrl := newRstController()
	ctrl.setObserved(fn, 1) // 1 of 2 containers survived the half-finished reclaim

	gates := NewGateRegistry(clk.Now)
	leases := NewLeaseRegistry(clk.Now)
	rec := newRstReconciler(clk, ctrl, []DeclaredFunction{{Name: fn, Policy: rstPolicy()}}, gates, leases, newRecordingMetrics())

	// First post-restart pass: the fresh process cannot know a reclaim was in
	// progress; it seeds and holds like for any surviving replica.
	res, err := rec.ReconcileOnce(ctx)
	if err != nil {
		t.Fatalf("seeding pass: %v", err)
	}
	if res.Reclaimed != 0 || ctrl.attemptCount(fn) != 0 {
		t.Fatalf("seeding pass must hold the half-reclaimed function: %+v", res)
	}

	// After the seeded idle window the reclaim completes idempotently.
	clk.Advance(rstPolicy().IdleDuration)
	res, err = rec.ReconcileOnce(ctx)
	if err != nil {
		t.Fatalf("completion pass: %v", err)
	}
	if res.Reclaimed != 1 || ctrl.attemptCount(fn) != 1 || ctrl.successCount(fn) != 1 {
		t.Fatalf("expected the interrupted reclaim to be completed by a second ReclaimToZero, got %+v (attempts=%d)", res, ctrl.attemptCount(fn))
	}
	if ctrl.observedCount(fn) != 0 {
		t.Fatalf("half-reclaimed function must converge to zero")
	}

	// Converged: further passes are no-ops.
	res, err = rec.ReconcileOnce(ctx)
	if err != nil {
		t.Fatalf("converged pass: %v", err)
	}
	if res.AlreadyZero != 1 || ctrl.attemptCount(fn) != 1 {
		t.Fatalf("converged function must not be reclaimed again, got %+v (attempts=%d)", res, ctrl.attemptCount(fn))
	}
}

// TestRestartReclaimFailureUnlocksGateAndRetries (RT-203c inverse): a reclaim
// whose provider call FAILS must release the gate (FinishReclaim(false)) so the
// next pass can retry, and the retry must eventually succeed.
func TestRestartReclaimFailureUnlocksGateAndRetries(t *testing.T) {
	const fn = "flaky"
	ctx := context.Background()
	clk := newRstClock(rstStart)
	ctrl := newRstController()
	ctrl.setObserved(fn, 1)
	ctrl.setReclaimErr(fn, errors.New("docker daemon busy"))

	gates := NewGateRegistry(clk.Now)
	leases := NewLeaseRegistry(clk.Now)
	rec := newRstReconciler(clk, ctrl, []DeclaredFunction{{Name: fn, Policy: rstPolicy()}}, gates, leases, newRecordingMetrics())

	if _, err := rec.ReconcileOnce(ctx); err != nil { // seeding pass
		t.Fatalf("seeding pass: %v", err)
	}
	clk.Advance(rstPolicy().IdleDuration)

	res, err := rec.ReconcileOnce(ctx)
	if err != nil {
		t.Fatalf("failing pass: %v", err)
	}
	if res.Reclaimed != 0 || ctrl.attemptCount(fn) != 1 {
		t.Fatalf("expected one failed reclaim attempt, got %+v (attempts=%d)", res, ctrl.attemptCount(fn))
	}
	if ctrl.observedCount(fn) != 1 {
		t.Fatalf("failed reclaim must leave the replica observable")
	}

	// The gate must have been unlocked by FinishReclaim(false): the very next
	// pass retries (a stuck gate would skip forever).
	res, err = rec.ReconcileOnce(ctx)
	if err != nil {
		t.Fatalf("retry pass: %v", err)
	}
	if res.Skipped != 0 || ctrl.attemptCount(fn) != 2 {
		t.Fatalf("failed reclaim must leave the gate unlocked for retry, got %+v (attempts=%d)", res, ctrl.attemptCount(fn))
	}

	// The transient failure clears; the retry converges.
	ctrl.setReclaimErr(fn, nil)
	res, err = rec.ReconcileOnce(ctx)
	if err != nil {
		t.Fatalf("recovery pass: %v", err)
	}
	if res.Reclaimed != 1 || ctrl.attemptCount(fn) != 3 || ctrl.successCount(fn) != 1 {
		t.Fatalf("retry must eventually succeed, got %+v (attempts=%d successes=%d)", res, ctrl.attemptCount(fn), ctrl.successCount(fn))
	}
	if ctrl.observedCount(fn) != 0 {
		t.Fatalf("function must converge to zero after the successful retry")
	}
}

// TestRestartLeaseGenerationSemantics (RT-203d): pins the generation-fence
// semantics around a restart. Design fact confirmed from gate.go: ONLY a cold
// start (AcquireColdStart leader election) bumps a function's generation;
// TryBeginReclaim/FinishReclaim never do. A restarted provider therefore starts
// every function back at generation 0, which has two consequences proven here:
//
//  1. a lease carried over from the previous process (higher generation than
//     the fresh registry's 0) is HONORED, not stale — the conservative-safe
//     direction: durable counts keep protecting the function;
//  2. staleness (lease generation < live) can only reappear after the first
//     post-restart cold start; the pure View staleness path drops such counts
//     to zero. (End-to-end reconciler cleanup of a stale lease, including the
//     StaleGenerationsCleaned metric and Forget, is covered by
//     TestReconcileStaleGenerationLeaseCannotKillBusyContainer.)
func TestRestartLeaseGenerationSemantics(t *testing.T) {
	const fn = "carried"
	ctx := context.Background()
	clk := newRstClock(rstStart)
	ctrl := newRstController()
	ctrl.setObserved(fn, 1)

	gates := NewGateRegistry(clk.Now)
	leases := NewLeaseRegistry(clk.Now)
	m := newRecordingMetrics()
	rec := newRstReconciler(clk, ctrl, []DeclaredFunction{{Name: fn, Policy: rstPolicy()}}, gates, leases, m)

	// The control plane still holds generation 5 from before the restart and
	// re-posts under it; the fresh registry is at generation 0.
	leases.Apply(rstLease(fn, 5, 2, 300))

	view := leases.View(fn, gates.Generation(fn))
	if view.StaleGeneration {
		t.Fatalf("a lease with generation ahead of the live one must be honored, not stale (post-restart safety)")
	}
	if view.DurableInFlight != 2 {
		t.Fatalf("honored lease DurableInFlight = %d, want 2", view.DurableInFlight)
	}

	// Even far past the idle window, the honored durable counts hold the
	// function.
	clk.Advance(10 * rstPolicy().IdleDuration)
	res, err := rec.ReconcileOnce(ctx)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.Reclaimed != 0 || ctrl.attemptCount(fn) != 0 {
		t.Fatalf("honored durable lease must protect the function, got %+v", res)
	}
	if m.staleCleaned[fn] != 0 {
		t.Fatalf("an honored (non-stale) lease must not be cleaned, got %d", m.staleCleaned[fn])
	}
	if m.decisions[fn] != ActionHold {
		t.Fatalf("decision = %v, want Hold", m.decisions[fn])
	}

	// The staleness path itself: once the live generation moves ahead (a
	// post-restart cold start), an older-generation lease contributes nothing.
	fenced := NewLeaseRegistry(clk.Now)
	fenced.Apply(rstLease("fenced", 0, 9, 300))
	v := fenced.View("fenced", 1)
	if !v.StaleGeneration {
		t.Fatalf("lease sampled under an older generation must be stale once the live generation advances")
	}
	if v.DurableInFlight != 0 {
		t.Fatalf("stale lease must contribute zero durable in-flight, got %d", v.DurableInFlight)
	}
}

// TestStormInvocationRacingReclaimDeterministic (RT-211): K=8 worker goroutines
// perform BeginInvocation/Release cycles fully sequenced by unbuffered
// command/ack channels, interleaved with ReconcileOnce calls at controlled
// points, over 120 scripted iterations (30 of each scenario). Invariants:
//
//   - ReclaimToZero is NEVER called while any invocation is in flight (checked
//     inside the controller callback at call time);
//   - an invocation injected BETWEEN the snapshot and TryBeginReclaim (via the
//     Decision metrics hook, which the reconciler calls in exactly that window)
//     forces the CAS to refuse: result.Skipped, no reclaim;
//   - an invocation injected DURING provider observation (before the snapshot,
//     via the controller's ObservedReplicas hook) is seen by the snapshot and
//     yields a plain Hold — decision-level protection, no skip;
//   - reclaims happen exactly and only in the scripted quiescent iterations.
//
// Every interleaving is explicitly sequenced by channels — no sleeps, no
// free-running loops — so the test is deterministic.
func TestStormInvocationRacingReclaimDeterministic(t *testing.T) {
	const fn = "storm"
	const workers = 8
	const iterations = 120
	ctx := context.Background()

	clk := newRstClock(rstStart)
	gates := NewGateRegistry(clk.Now)
	leases := NewLeaseRegistry(clk.Now)
	ctrl := newRstController()
	ctrl.setObserved(fn, 1)
	policy := rstPolicy()

	// The load-bearing invariant, checked at ReclaimToZero call time.
	var violations []string
	ctrl.onReclaim = func(f string) {
		if n := gates.InFlight(f); n != 0 {
			violations = append(violations, fmt.Sprintf("ReclaimToZero(%s) called with %d in-flight invocations", f, n))
		}
	}

	sm := &stormMetrics{}
	rec := newRstReconciler(clk, ctrl, []DeclaredFunction{{Name: fn, Policy: policy}}, gates, leases, sm)

	// Channel-sequenced workers: each command is acknowledged before the driver
	// takes its next step, so the driver script defines the exact interleaving.
	type stormWorker struct {
		cmds chan bool // true = BeginInvocation, false = Release
		acks chan struct{}
	}
	ws := make([]stormWorker, workers)
	var wg sync.WaitGroup
	for i := range ws {
		ws[i] = stormWorker{cmds: make(chan bool), acks: make(chan struct{})}
		wg.Add(1)
		go func(w stormWorker) {
			defer wg.Done()
			var tok *InvocationToken
			for begin := range w.cmds {
				if begin {
					tok = gates.BeginInvocation(fn)
				} else if tok != nil {
					tok.Release()
					tok = nil
				}
				w.acks <- struct{}{}
			}
		}(ws[i])
	}
	begin := func(i int) { ws[i].cmds <- true; <-ws[i].acks }
	release := func(i int) { ws[i].cmds <- false; <-ws[i].acks }
	pass := func(iter int) ReconcileResult {
		res, err := rec.ReconcileOnce(ctx)
		if err != nil {
			t.Fatalf("iteration %d: reconcile: %v", iter, err)
		}
		return res
	}

	expectedReclaims := 0
	expectedSkips := 0
	observedSkips := 0

	for iter := 0; iter < iterations; iter++ {
		switch iter % 4 {
		case 0:
			// Busy burst: with any invocation in flight, reclaim is impossible
			// no matter how much wall-clock time has passed.
			for i := 0; i < workers; i++ {
				begin(i)
			}
			clk.Advance(2 * policy.IdleDuration)
			if res := pass(iter); res.Reclaimed != 0 {
				t.Fatalf("iteration %d: reclaimed with %d invocations in flight: %+v", iter, workers, res)
			}
			for i := 0; i < workers/2; i++ {
				release(i)
			}
			if res := pass(iter); res.Reclaimed != 0 {
				t.Fatalf("iteration %d: reclaimed with %d invocations still in flight: %+v", iter, workers/2, res)
			}
			for i := workers / 2; i < workers; i++ {
				release(i)
			}
			// Releases just marked activity: the stable idle window must hold
			// the function on the immediately following pass (SZ-08).
			if res := pass(iter); res.Reclaimed != 0 {
				t.Fatalf("iteration %d: reclaimed immediately after last release (stable window violated): %+v", iter, res)
			}

		case 1:
			// Quiescent reclaim: a full idle window with no work of any kind.
			ctrl.setObserved(fn, 1)
			clk.Advance(policy.IdleDuration)
			res := pass(iter)
			if res.Reclaimed != 1 {
				t.Fatalf("iteration %d: expected exactly one reclaim when quiescent, got %+v", iter, res)
			}
			expectedReclaims++

		case 2:
			// RT-211 exact window: an invocation begins BETWEEN the snapshot
			// and TryBeginReclaim. The Decision metrics hook runs in precisely
			// that window, so the CAS must refuse and the pass must count a
			// skip.
			ctrl.setObserved(fn, 1)
			clk.Advance(policy.IdleDuration)
			var injected *InvocationToken
			sm.onDecision = func(f string, a Action) {
				if f == fn && a == ActionScaleToZero && injected == nil {
					injected = gates.BeginInvocation(fn)
				}
			}
			res := pass(iter)
			sm.onDecision = nil
			if injected == nil {
				t.Fatalf("iteration %d: expected a scale-to-zero decision to inject against", iter)
			}
			if res.Reclaimed != 0 || res.Skipped != 1 {
				t.Fatalf("iteration %d: invocation between snapshot and CAS must skip the reclaim, got %+v", iter, res)
			}
			observedSkips += res.Skipped
			expectedSkips++
			injected.Release()

		case 3:
			// Decision-level protection: an invocation that begins DURING the
			// provider observation is already visible to the snapshot, so the
			// decider holds — no reclaim attempt, no skip.
			ctrl.setObserved(fn, 1)
			clk.Advance(policy.IdleDuration)
			var injected *InvocationToken
			ctrl.onObserve = func(f string) {
				if f == fn && injected == nil {
					injected = gates.BeginInvocation(fn)
				}
			}
			res := pass(iter)
			ctrl.onObserve = nil
			if injected == nil {
				t.Fatalf("iteration %d: observation hook did not fire", iter)
			}
			if res.Reclaimed != 0 || res.Skipped != 0 || res.Held != 1 {
				t.Fatalf("iteration %d: pre-snapshot invocation must yield a plain Hold, got %+v", iter, res)
			}
			injected.Release()
		}
	}

	for i := range ws {
		close(ws[i].cmds)
	}
	wg.Wait()

	if len(violations) != 0 {
		t.Fatalf("in-flight invariant violated %d time(s): %v", len(violations), violations)
	}
	if got := ctrl.attemptCount(fn); got != expectedReclaims {
		t.Fatalf("ReclaimToZero attempts = %d, want exactly %d (reclaims only in scripted quiescent iterations)", got, expectedReclaims)
	}
	if got := ctrl.successCount(fn); got != expectedReclaims {
		t.Fatalf("ReclaimToZero successes = %d, want %d", got, expectedReclaims)
	}
	if observedSkips != expectedSkips {
		t.Fatalf("skipped reclaims = %d, want %d", observedSkips, expectedSkips)
	}
}

// TestStormLateLeaseAfterReclaimHoldsNextPass (RT-211): a durable-work lease
// for generation G arrives immediately AFTER a reclaim completed under G.
// Reclaim does not bump the generation (only a cold start does — pinned here),
// so the lease is honored by design: its counts protect the NEXT decision. The
// next pass must HOLD (with a >=1 desired-replica signal from the decider) and
// must not flap into another reclaim; actual scale-up belongs to the gateway
// cold-start path, so the reconciler makes no scale-up call.
func TestStormLateLeaseAfterReclaimHoldsNextPass(t *testing.T) {
	const fn = "late"
	ctx := context.Background()
	clk := newRstClock(rstStart)
	ctrl := newRstController()
	ctrl.setObserved(fn, 1)

	gates := NewGateRegistry(clk.Now)
	leases := NewLeaseRegistry(clk.Now)
	m := newRecordingMetrics()
	rec := newRstReconciler(clk, ctrl, []DeclaredFunction{{Name: fn, Policy: rstPolicy()}}, gates, leases, m)

	if _, err := rec.ReconcileOnce(ctx); err != nil { // seeding pass
		t.Fatalf("seeding pass: %v", err)
	}
	genBefore := gates.Generation(fn)

	clk.Advance(rstPolicy().IdleDuration)
	res, err := rec.ReconcileOnce(ctx)
	if err != nil {
		t.Fatalf("reclaim pass: %v", err)
	}
	if res.Reclaimed != 1 || ctrl.attemptCount(fn) != 1 {
		t.Fatalf("expected the idle function to be reclaimed, got %+v", res)
	}
	if got := gates.Generation(fn); got != genBefore {
		t.Fatalf("reclaim must not bump the generation (only a cold start does), got %d want %d", got, genBefore)
	}

	// The control plane sampled its state before it learned of the scale-down
	// and posts a lease under the SAME generation right after the reclaim.
	leases.Apply(rstLease(fn, genBefore, 3, 300))

	res, err = rec.ReconcileOnce(ctx)
	if err != nil {
		t.Fatalf("post-lease pass: %v", err)
	}
	if res.Held != 1 || res.Reclaimed != 0 || res.AlreadyZero != 0 || res.Skipped != 0 {
		t.Fatalf("post-reclaim lease with durable counts must HOLD the next pass, got %+v", res)
	}
	if m.decisions[fn] != ActionHold {
		t.Fatalf("decision = %v, want Hold", m.decisions[fn])
	}
	if ctrl.attemptCount(fn) != 1 {
		t.Fatalf("no further reclaim attempt may happen, got %d", ctrl.attemptCount(fn))
	}
	if len(ctrl.warmCalls) != 0 {
		t.Fatalf("reconciler must not scale up on Hold (cold start belongs to the gateway path), got %v", ctrl.warmCalls)
	}

	// Pin the decider's signal for the scale-up path: durable work at zero
	// replicas asks for at least one replica while holding.
	dec := IdleDecider{}.Decide(clk.Now(), rstPolicy(), ActivitySnapshot{
		DurableInFlight:  3,
		LastActivity:     clk.Now(),
		ObservedReplicas: 0,
	})
	if dec.Action != ActionHold || dec.DesiredReplicas != 1 {
		t.Fatalf("durable work at zero replicas must signal Hold with DesiredReplicas=1, got %+v", dec)
	}
}
