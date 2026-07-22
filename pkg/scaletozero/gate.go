package scaletozero

import (
	"context"
	"sync"
	"time"
)

// GateRegistry holds one coordination gate per function. A single gate unifies
// the three concurrency invariants the idle scaler needs:
//
//   - Authoritative gateway in-flight accounting (SZ-03): every invocation is
//     counted for its full lifetime, so a long-running / async / retried /
//     cancelling request is visible to the reaper.
//   - A generation fence (SZ-01/SZ-08): each cold start bumps the function's
//     generation, invalidating any idle decision the reaper computed under the
//     old generation.
//   - One reconcile operation at a time (SZ-02): concurrent requests to a
//     zero-replica function elect a single leader that performs one
//     scale-from-zero; the rest wait on readiness. Reclaim and cold start are
//     mutually exclusive per function.
//
// The registry is safe for concurrent use and depends only on the standard
// library.
type GateRegistry struct {
	mu    sync.Mutex
	gates map[string]*functionGate
	now   func() time.Time
}

// NewGateRegistry creates an empty registry. Pass nil clock for time.Now.
func NewGateRegistry(clock func() time.Time) *GateRegistry {
	if clock == nil {
		clock = time.Now
	}
	return &GateRegistry{
		gates: make(map[string]*functionGate),
		now:   clock,
	}
}

type functionGate struct {
	mu           sync.Mutex
	inFlight     int
	generation   uint64
	lastActivity time.Time
	// scaleDone is non-nil while any scale operation (reclaim or cold start)
	// is running; it is closed when the operation completes so waiters wake.
	scaleDone chan struct{}
	// cold is non-nil while a cold start is running (a subset of a scale op).
	cold  *coldStartOp
	ready bool
	// reclaiming is true only between a successful TryBeginReclaim and its
	// FinishReclaim, so an unpaired/spurious FinishReclaim can never steal a
	// cold start's channel or demote readiness (RT-218).
	reclaiming bool
}

type coldStartOp struct {
	done chan struct{}
	err  error
	// completed guards Complete against double invocation: the second call is
	// a no-op instead of a close-of-closed-channel panic (RT-218).
	completed bool
}

func (reg *GateRegistry) gate(fn string) *functionGate {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	g, ok := reg.gates[fn]
	if !ok {
		g = &functionGate{}
		reg.gates[fn] = g
	}
	return g
}

// InvocationToken represents one in-flight invocation. Release MUST be called
// (typically via defer) exactly once when the invocation completes.
type InvocationToken struct {
	reg            *GateRegistry
	fn             string
	released       bool
	reclaimAtBegin bool
}

// BeginInvocation registers a new in-flight invocation for fn. It increments
// the authoritative in-flight count BEFORE the caller inspects replica
// availability, so a reaper cannot begin a reclaim between the check and the
// route (SZ-01).
func (reg *GateRegistry) BeginInvocation(fn string) *InvocationToken {
	g := reg.gate(fn)
	g.mu.Lock()
	g.inFlight++
	g.lastActivity = reg.now()
	// A reclaim (scale op with no cold start) is in progress if scaleDone is
	// set but cold is not.
	reclaiming := g.scaleDone != nil && g.cold == nil
	g.mu.Unlock()
	return &InvocationToken{reg: reg, fn: fn, reclaimAtBegin: reclaiming}
}

// ReclaimInProgress reports whether a reclaim was underway when the invocation
// began; the caller should then force a cold start rather than route to a
// container being torn down.
func (t *InvocationToken) ReclaimInProgress() bool { return t.reclaimAtBegin }

// Release marks the invocation complete and decrements the in-flight count.
func (t *InvocationToken) Release() {
	if t == nil || t.released {
		return
	}
	t.released = true
	g := t.reg.gate(t.fn)
	g.mu.Lock()
	if g.inFlight > 0 {
		g.inFlight--
	}
	g.lastActivity = t.reg.now()
	g.mu.Unlock()
}

// InFlight returns the authoritative gateway in-flight count for fn.
func (reg *GateRegistry) InFlight(fn string) int {
	g := reg.gate(fn)
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.inFlight
}

// LastActivity returns the last time an invocation began or ended for fn.
func (reg *GateRegistry) LastActivity(fn string) time.Time {
	g := reg.gate(fn)
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.lastActivity
}

// Generation returns the current generation fence for fn.
func (reg *GateRegistry) Generation(fn string) uint64 {
	g := reg.gate(fn)
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.generation
}

// MarkActivity records external activity (e.g. a durable job the control plane
// reports) so the idle timer is not fooled by a lack of gateway HTTP traffic.
func (reg *GateRegistry) MarkActivity(fn string, at time.Time) {
	g := reg.gate(fn)
	g.mu.Lock()
	if at.After(g.lastActivity) {
		g.lastActivity = at
	}
	g.mu.Unlock()
}

// ColdStart is the result of AcquireColdStart.
type ColdStart struct {
	// Leader is true for exactly one caller among concurrent cold starts.
	Leader bool
	// Wait blocks (followers) until the leader's cold start finishes and
	// returns the leader's result. For the leader it returns immediately.
	Wait func(ctx context.Context) error
	// Complete is called by the leader once scale-from-zero + readiness
	// finished (nil) or failed (err). No-op for followers.
	Complete func(err error)
}

// AcquireColdStart elects a single leader to scale fn from zero. Concurrent
// callers at zero get Leader=false and wait on the leader (SZ-02). If a reclaim
// is currently running for fn, AcquireColdStart waits for it to finish before
// electing a leader, so cold start and reclaim never overlap. Electing a leader
// bumps the generation, fencing any stale reaper decision (SZ-01).
//
// A caller that arrives once the replica is already ready — including a LATE
// caller that shows up AFTER the in-progress leader finished — must NOT elect a
// second leader (CV-07). It observes readiness and proceeds against the ready
// replica with Leader=false and an immediately-returning Wait. Readiness is
// re-checked under the gate mutex AFTER the reclaim check, so a cold start can
// never race an in-flight reclaim (which still holds ready=true until it
// commits): the reclaim path is waited out first, and only once the replica is
// genuinely gone (ready=false again via FinishReclaim) does a fresh leader get
// elected. This makes leader election and the readiness transition race-free.
func (reg *GateRegistry) AcquireColdStart(fn string) ColdStart {
	// context.Background never cancels, so the error path of the ctx-aware
	// variant is unreachable here and the legacy blocking semantics are kept.
	cs, _ := reg.AcquireColdStartCtx(context.Background(), fn)
	return cs
}

// AcquireColdStartCtx is AcquireColdStart with cancellation: a caller parked
// behind an in-progress reclaim honors ctx and returns ctx.Err() instead of
// blocking indefinitely on a hung scale operation (RT-213 cancellation).
func (reg *GateRegistry) AcquireColdStartCtx(ctx context.Context, fn string) (ColdStart, error) {
	g := reg.gate(fn)
	for {
		g.mu.Lock()
		if g.cold != nil {
			// A cold start is in progress; follow the single in-flight leader.
			op := g.cold
			g.mu.Unlock()
			return ColdStart{
				Leader:   false,
				Wait:     waitColdStart(op),
				Complete: func(error) {},
			}, nil
		}
		if g.scaleDone != nil {
			// A reclaim (a scale op with no cold start) is running; wait for it
			// to finish, then retry. This is checked BEFORE the ready gate so a
			// caller arriving mid-reclaim never routes to a replica being torn
			// down (the reclaim holds ready=true until it commits ready=false).
			ch := g.scaleDone
			g.mu.Unlock()
			select {
			case <-ch:
			case <-ctx.Done():
				return ColdStart{}, ctx.Err()
			}
			continue
		}
		if g.ready {
			// The replica is already up and has not been reclaimed since. A late
			// caller must NOT trigger a redundant cold start; it proceeds against
			// the ready replica (CV-07).
			g.mu.Unlock()
			return ColdStart{
				Leader:   false,
				Wait:     func(context.Context) error { return nil },
				Complete: func(error) {},
			}, nil
		}
		op := &coldStartOp{done: make(chan struct{})}
		g.cold = op
		g.scaleDone = make(chan struct{})
		g.generation++
		g.mu.Unlock()
		return ColdStart{
			Leader:   true,
			Wait:     func(context.Context) error { return nil },
			Complete: reg.completeColdStart(g, op),
		}, nil
	}
}

// ReportZeroObserved tells the gate that the caller observed ZERO running
// replicas for fn while the gate believed it ready, under generation gen. If
// gen is still current, the gate is Ready, and no scale operation is running,
// readiness is demoted so the next AcquireColdStart elects a fresh leader —
// this is the crash-recovery path (RT-213): a container that dies OUTSIDE a
// reclaim (crash, external rm, daemon restart) would otherwise leave ready
// stuck true and every invocation routing to nothing. A stale gen (the fence
// moved since the observation) or an in-progress scale op rejects the
// demotion, so a slow observer can never demote readiness established by a
// newer cold start.
func (reg *GateRegistry) ReportZeroObserved(fn string, gen uint64) bool {
	g := reg.gate(fn)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.generation != gen || g.scaleDone != nil || g.cold != nil || !g.ready {
		return false
	}
	g.ready = false
	return true
}

// Ready reports whether the gate currently believes fn has a ready replica
// (i.e. the last cold start succeeded and no reclaim has completed since).
func (reg *GateRegistry) Ready(fn string) bool {
	g := reg.gate(fn)
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.ready
}

func (reg *GateRegistry) completeColdStart(g *functionGate, op *coldStartOp) func(error) {
	return func(err error) {
		g.mu.Lock()
		if op.completed {
			// Second Complete on the same op: no-op (RT-218) — never close a
			// closed channel or clobber a newer operation's state.
			g.mu.Unlock()
			return
		}
		op.completed = true
		op.err = err
		var ch chan struct{}
		if g.cold == op {
			// Only clear gate state if this op still owns it; a stale Complete
			// must never tear down a newer cold start's bookkeeping.
			g.cold = nil
			ch = g.scaleDone
			g.scaleDone = nil
			if err == nil {
				g.ready = true
				g.lastActivity = reg.now()
			}
		}
		g.mu.Unlock()
		close(op.done)
		if ch != nil {
			close(ch)
		}
	}
}

func waitColdStart(op *coldStartOp) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		select {
		case <-op.done:
			return op.err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// TryBeginReclaim attempts to take exclusive ownership to reclaim fn to zero. It
// succeeds only if the live generation still matches gen (no cold start raced),
// no work is in flight, and no other scale operation is running. On success the
// caller MUST call FinishReclaim. This is the compare-and-swap that makes the
// idle reaper unable to terminate a container with in-flight or about-to-run
// work (SZ-01/SZ-08).
func (reg *GateRegistry) TryBeginReclaim(fn string, gen uint64) bool {
	g := reg.gate(fn)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.scaleDone != nil || g.cold != nil {
		return false
	}
	if g.generation != gen {
		return false
	}
	if g.inFlight > 0 {
		return false
	}
	g.scaleDone = make(chan struct{})
	g.reclaiming = true
	return true
}

// FinishReclaim releases reclaim ownership taken by TryBeginReclaim. Pass
// reclaimed=true when the function was actually scaled to zero. An unpaired
// call (no TryBeginReclaim succeeded) is a no-op, so a buggy caller can never
// steal a cold start's channel or spuriously demote readiness (RT-218).
func (reg *GateRegistry) FinishReclaim(fn string, reclaimed bool) {
	g := reg.gate(fn)
	g.mu.Lock()
	if !g.reclaiming {
		g.mu.Unlock()
		return
	}
	g.reclaiming = false
	ch := g.scaleDone
	g.scaleDone = nil
	if reclaimed {
		g.ready = false
	}
	g.mu.Unlock()
	if ch != nil {
		close(ch)
	}
}

// Forget removes a function's gate entirely (used when a function is deleted so
// its generation/accounting state does not leak).
func (reg *GateRegistry) Forget(fn string) {
	reg.mu.Lock()
	delete(reg.gates, fn)
	reg.mu.Unlock()
}

// Functions returns the names of all functions with a live gate.
func (reg *GateRegistry) Functions() []string {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	out := make([]string, 0, len(reg.gates))
	for fn := range reg.gates {
		out = append(out, fn)
	}
	return out
}
