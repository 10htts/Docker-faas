package scaletozero

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// ReplicaController is the provider surface the idle reconciler drives. The
// Docker provider implements it; tests use a fake. It is intentionally narrow
// so the reconciler never holds a Docker client directly.
type ReplicaController interface {
	// ObservedReplicas returns the provider-observed RUNNING replica count for
	// a function (SZ-09: the decision uses real observation, not desired
	// state).
	ObservedReplicas(ctx context.Context, fn string) (int, error)
	// ReclaimToZero stops and removes all replicas of fn and reclaims its
	// resources (containers, per-function network). It is idempotent and
	// returns a report proving what was reclaimed (SZ-09).
	ReclaimToZero(ctx context.Context, fn string) (ReclaimReport, error)
	// ObservedFunctions returns the set of function names the provider has
	// containers for, used for orphan detection after a restart (SZ-07).
	ObservedFunctions(ctx context.Context) ([]string, error)
	// EnsureWarmMinimum scales fn up to at least min replicas if below it
	// (SZ-04 warm exception).
	EnsureWarmMinimum(ctx context.Context, fn string, min int) error
}

// ReclaimReport records the resources released when a function is reclaimed. It
// is proof of reclamation beyond "container stopped" (SZ-09).
type ReclaimReport struct {
	ContainersRemoved int
	NetworksRemoved   int
	MemoryBytesFreed  int64
	NanoCPUsFreed     int64
}

// PolicySource yields the declared functions and their idle policies. Backed by
// the provider store, so desired state is reconstructable after a gateway
// restart without a second policy store (SZ-07/SZ-11).
type PolicySource interface {
	DeclaredFunctions(ctx context.Context) ([]DeclaredFunction, error)
}

// DeclaredFunction pairs a function name with its idle policy.
type DeclaredFunction struct {
	Name   string
	Policy Policy
}

// MetricsSink receives idle-reconciler observations. main wires the Prometheus
// recorders; tests can pass a no-op or a recording sink.
type MetricsSink interface {
	ObservedReplicas(fn string, replicas int)
	Decision(fn string, action Action)
	IdleReclamation(fn string, report ReclaimReport)
	StaleGenerationsCleaned(fn string, count int)
	ReconcilePass(reclaimed, held, keptWarm, orphans, skipped int)
}

// NopMetrics is a MetricsSink that discards everything.
type NopMetrics struct{}

func (NopMetrics) ObservedReplicas(string, int)          {}
func (NopMetrics) Decision(string, Action)               {}
func (NopMetrics) IdleReclamation(string, ReclaimReport) {}
func (NopMetrics) StaleGenerationsCleaned(string, int)   {}
func (NopMetrics) ReconcilePass(int, int, int, int, int) {}

// IdleReconciler converges declared function idle policy against actual
// provider state. It owns the single reconcile operation per function (SZ-02)
// and refuses to reclaim work that is admitted, queued, running, or in-flight
// (SZ-01/SZ-03).
type IdleReconciler struct {
	controller ReplicaController
	policies   PolicySource
	gates      *GateRegistry
	leases     *LeaseRegistry
	decider    IdleDecider
	metrics    MetricsSink
	logger     *logrus.Logger
	interval   time.Duration
	now        func() time.Time

	// warnMu guards warnedShortIdleWindow: the once-per-function-per-process
	// rate limit for the short-idle-window config-safety warning.
	warnMu                sync.Mutex
	warnedShortIdleWindow map[string]bool

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// ReconcilerConfig configures an IdleReconciler.
type ReconcilerConfig struct {
	Controller ReplicaController
	Policies   PolicySource
	Gates      *GateRegistry
	Leases     *LeaseRegistry
	Metrics    MetricsSink
	Logger     *logrus.Logger
	Interval   time.Duration
	Clock      func() time.Time
}

// NewIdleReconciler builds a reconciler from config.
func NewIdleReconciler(cfg ReconcilerConfig) *IdleReconciler {
	clock := cfg.Clock
	if clock == nil {
		clock = time.Now
	}
	var sink MetricsSink = cfg.Metrics
	if sink == nil {
		sink = NopMetrics{}
	}
	logger := cfg.Logger
	if logger == nil {
		logger = logrus.New()
	}
	return &IdleReconciler{
		controller:            cfg.Controller,
		policies:              cfg.Policies,
		gates:                 cfg.Gates,
		leases:                cfg.Leases,
		metrics:               sink,
		logger:                logger,
		interval:              cfg.Interval,
		now:                   clock,
		warnedShortIdleWindow: make(map[string]bool),
		stopCh:                make(chan struct{}),
	}
}

// ReconcileResult summarizes one reconcile pass.
type ReconcileResult struct {
	Reclaimed      int
	Held           int
	KeptWarm       int
	OrphansCleaned int
	Skipped        int
	AlreadyZero    int
}

// safeReclaim takes reclaim ownership of fn and reclaims it to zero, and
// GUARANTEES FinishReclaim runs even if the controller's ReclaimToZero panics
// (a raw Docker SDK call). Without the deferred FinishReclaim a panic would
// leave the gate's scale lock held forever — the function could never cold-start
// again — and without the recover it would crash the whole gateway process
// (every function's gateway, not just this one). A panic is converted into an
// error so the pass continues to the next function. began=false means ownership
// could not be taken (raced by an invocation / stale generation / in-flight).
func (r *IdleReconciler) safeReclaim(ctx context.Context, fn string, gen uint64) (report ReclaimReport, began bool, err error) {
	if !r.gates.TryBeginReclaim(fn, gen) {
		return ReclaimReport{}, false, nil
	}
	began = true
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("reclaim of %s panicked: %v", fn, rec)
			r.logger.Errorf("idle reconcile: %v", err)
		}
		r.gates.FinishReclaim(fn, err == nil)
	}()
	report, err = r.controller.ReclaimToZero(ctx, fn)
	return report, began, err
}

// ReconcileOnce runs a single idempotent convergence pass over all declared
// functions and cleans orphan containers. It is safe to call repeatedly and on
// startup for restart convergence (SZ-07). A panic anywhere in the pass (e.g. a
// Docker SDK call) is recovered and returned as an error rather than crashing
// the gateway; the periodic loop logs it and retries on the next tick.
func (r *IdleReconciler) ReconcileOnce(ctx context.Context) (result ReconcileResult, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("idle reconcile pass panicked: %v", rec)
			r.logger.Errorf("%v", err)
		}
	}()
	now := r.now()

	declared, derr := r.policies.DeclaredFunctions(ctx)
	if derr != nil {
		return ReconcileResult{}, derr
	}

	result = ReconcileResult{}
	declaredSet := make(map[string]bool, len(declared))

	for _, df := range declared {
		declaredSet[df.Name] = true

		observed, err := r.controller.ObservedReplicas(ctx, df.Name)
		if err != nil {
			r.logger.Warnf("idle reconcile: observe replicas for %s: %v", df.Name, err)
			continue
		}
		r.metrics.ObservedReplicas(df.Name, observed)

		gen := r.gates.Generation(df.Name)
		view := r.leases.View(df.Name, gen)
		if view.StaleGeneration {
			r.metrics.StaleGenerationsCleaned(df.Name, 1)
			r.leases.Forget(df.Name)
		}

		// Config safety: warn (once per function per process) when the idle
		// window leaves little margin over the durable-lease TTL.
		r.maybeWarnShortIdleWindow(df.Name, df.Policy)

		// RT-203 restart convergence: gates and leases are in-memory, so after
		// a gateway restart LastActivity is zero even for functions that still
		// have live containers. The provider cannot know how long such a
		// function was really idle before the restart, so it seeds the idle
		// clock at the FIRST post-restart observation of a running replica.
		// This guarantees both restart-safety properties:
		//   (a) the function is NEVER reclaimed before one FULL idle window
		//       has elapsed after the restart, giving the control plane
		//       (default lease TTL 30s, re-posted continuously) ample time to
		//       re-assert durable work via a fresh lease; and
		//   (b) a genuinely idle function still CONVERGES to reclamation once
		//       that window passes with no activity, instead of being held
		//       forever by the "no recorded activity" conservative rule.
		// The seeding pass itself always HOLDs: at the seed instant the
		// function has been idle for exactly zero of the required window.
		if observed > 0 && r.gates.LastActivity(df.Name).IsZero() {
			r.gates.MarkActivity(df.Name, now)
		}

		// Fold durable activity into the gate so the idle window stays
		// anchored to the control plane's LAST renewal. View reports the
		// last-activity timestamp even for an EXPIRED lease (expiry drops only
		// the counts — the SZ-08 fail-safe): without that, the first pass
		// after expiry could reclaim a function whose control plane reported
		// activity only seconds earlier, violating the stable idle window.
		// MarkActivity takes the max, so this never moves the idle clock
		// backwards; folding makes the anchor survive a later lease Forget.
		if !view.LastActivity.IsZero() {
			r.gates.MarkActivity(df.Name, view.LastActivity)
		}

		snap := ActivitySnapshot{
			GatewayInFlight:  r.gates.InFlight(df.Name),
			DurableInFlight:  view.DurableInFlight,
			LastActivity:     latest(r.gates.LastActivity(df.Name), view.LastActivity),
			ObservedReplicas: observed,
		}

		decision := r.decider.Decide(now, df.Policy, snap)
		r.metrics.Decision(df.Name, decision.Action)

		switch decision.Action {
		case ActionScaleToZero:
			if observed == 0 {
				result.AlreadyZero++
				continue
			}
			// Compare-and-swap the generation and re-check in-flight under the
			// gate lock (inside safeReclaim). If an invocation raced in (bumping
			// the generation or incrementing in-flight) the reclaim is refused
			// (SZ-01/SZ-08). safeReclaim guarantees FinishReclaim + panic safety.
			report, began, rerr := r.safeReclaim(ctx, df.Name, gen)
			if !began {
				result.Skipped++
				r.logger.Debugf("idle reconcile: skip reclaim of %s (raced by invocation)", df.Name)
				continue
			}
			if rerr != nil {
				r.logger.Errorf("idle reconcile: reclaim %s: %v", df.Name, rerr)
				continue
			}
			r.metrics.IdleReclamation(df.Name, report)
			r.metrics.ObservedReplicas(df.Name, 0)
			result.Reclaimed++
			r.logger.Infof("idle reconcile: reclaimed %s to zero (%s)", df.Name, decision.Reason)

		case ActionKeepWarm:
			// Bounds safety: a warm minimum above MaxReplicas must never make
			// keep-warm scale PAST the configured cap. Enforce the clamped
			// effective minimum (the decider mirrors this in DesiredReplicas)
			// and surface the misconfiguration once per pass.
			effectiveMin := df.Policy.EffectiveWarmMinimum()
			if df.Policy.MinReplicas > effectiveMin {
				r.logger.Warnf("idle reconcile: %s min replicas %d exceeds max replicas %d; keeping warm at %d", df.Name, df.Policy.MinReplicas, df.Policy.MaxReplicas, effectiveMin)
			}
			if observed < effectiveMin {
				if err := r.controller.EnsureWarmMinimum(ctx, df.Name, effectiveMin); err != nil {
					r.logger.Errorf("idle reconcile: ensure warm minimum for %s: %v", df.Name, err)
				}
			}
			result.KeptWarm++

		default:
			result.Held++
		}
	}

	// Orphan cleanup: containers that belong to no declared function are removed
	// and their accounting state is forgotten (SZ-07 restart convergence,
	// SZ-09 stale-generation cleanup).
	observedFns, err := r.controller.ObservedFunctions(ctx)
	if err != nil {
		r.logger.Warnf("idle reconcile: list observed functions: %v", err)
	} else {
		for _, fn := range observedFns {
			if declaredSet[fn] {
				continue
			}
			report, began, rerr := r.safeReclaim(ctx, fn, r.gates.Generation(fn))
			if !began {
				result.Skipped++
				continue
			}
			if rerr != nil {
				r.logger.Errorf("idle reconcile: reclaim orphan %s: %v", fn, rerr)
				continue
			}
			r.metrics.IdleReclamation(fn, report)
			r.metrics.StaleGenerationsCleaned(fn, 1)
			r.gates.Forget(fn)
			r.leases.Forget(fn)
			result.OrphansCleaned++
			r.logger.Infof("idle reconcile: cleaned orphan function %s", fn)
		}
	}

	r.metrics.ReconcilePass(result.Reclaimed, result.Held, result.KeptWarm, result.OrphansCleaned, result.Skipped)
	return result, nil
}

// maybeWarnShortIdleWindow logs a one-line warning — once per function name per
// process (reconciler lifetime) — when an enabled policy's idle window is
// dangerously close to the durable-lease TTL. Leases are the control plane's
// ONLY way to assert durable work; below 3x the default TTL (90s) a single
// delayed or lost renewal leaves little margin before a busy function starts to
// look reclaim-eligible.
func (r *IdleReconciler) maybeWarnShortIdleWindow(fn string, p Policy) {
	if !p.Enabled || p.IdleDuration >= 3*DefaultLeaseTTL {
		return
	}
	r.warnMu.Lock()
	warned := r.warnedShortIdleWindow[fn]
	r.warnedShortIdleWindow[fn] = true
	r.warnMu.Unlock()
	if warned {
		return
	}
	r.logger.Warnf("idle reconcile: %s idle window %s is below 3x the default lease TTL (%s); a delayed durable-work lease renewal could be mistaken for idleness — consider an idle window of at least %s", fn, p.IdleDuration, DefaultLeaseTTL, 3*DefaultLeaseTTL)
}

// StartPeriodic runs ReconcileOnce on the configured interval until Stop or ctx
// cancellation.
func (r *IdleReconciler) StartPeriodic(ctx context.Context) {
	if r.interval <= 0 {
		return
	}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()

		r.logger.Infof("Idle scale-to-zero reconciler started (interval: %s)", r.interval)
		for {
			select {
			case <-ctx.Done():
				r.logger.Info("Idle reconciler stopped (context cancelled)")
				return
			case <-r.stopCh:
				r.logger.Info("Idle reconciler stopped")
				return
			case <-ticker.C:
				if _, err := r.ReconcileOnce(ctx); err != nil {
					r.logger.Errorf("Periodic idle reconcile failed: %v", err)
				}
			}
		}
	}()
}

// Stop terminates the periodic loop.
func (r *IdleReconciler) Stop() {
	select {
	case <-r.stopCh:
	default:
		close(r.stopCh)
	}
	r.wg.Wait()
}
