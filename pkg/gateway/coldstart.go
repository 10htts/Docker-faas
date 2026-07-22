package gateway

import (
	"context"
	"fmt"
	"time"

	"github.com/docker-faas/docker-faas/pkg/metrics"
	"github.com/docker-faas/docker-faas/pkg/types"
)

// coldStartMaxAttempts bounds the acquire/verify/retry loop in
// ensureReadyFromZero. Attempt 1 discovers the state (follower of an existing
// leader, stale-ready crash detection, or leader election); attempts 2..N cover
// failed-start retry — a follower whose leader failed, or a caller that demoted
// stale readiness, re-acquires and may elect itself the fresh leader (RT-213).
const coldStartMaxAttempts = 3

// trackInvocation registers a new in-flight invocation for a function when idle
// scale-to-zero is wired, so the idle reaper cannot reclaim a function while a
// request (sync or async, including long-running/retried/cancelling work) is
// executing (SZ-01/SZ-03). It returns a release func (always safe to call) and
// whether a reclaim was in progress at begin time.
func (g *Gateway) trackInvocation(functionName string) (release func(), reclaimInProgress bool) {
	if g.scale == nil || g.scale.gates == nil {
		return func() {}, false
	}
	tok := g.scale.gates.BeginInvocation(functionName)
	return tok.Release, tok.ReclaimInProgress()
}

// ensureReadyFromZero brings a function up from zero. When idle scale-to-zero is
// wired it uses single-leader cold start so N concurrent requests at zero create
// exactly ONE container and the rest wait on readiness (SZ-02). Otherwise it
// falls back to the legacy direct scale-from-zero path.
//
// The loop is the deterministic client of the gate state machine:
//   - Leader: performs the one scale-from-zero + readiness wait, then Complete.
//   - Follower: waits for the leader's result with ctx cancellation; if the
//     leader FAILED and our own ctx is still alive, retry — the re-acquire
//     elects a fresh leader (failed-start retry).
//   - Ready (no leader needed): verify a running replica actually exists; if
//     the container died outside a reclaim the gate's readiness is stale —
//     demote it under the observed generation fence (ReportZeroObserved) and
//     retry, which elects a fresh leader (RT-213 crash recovery). The fence
//     means a slow health check can never demote readiness established by a
//     newer cold start.
func (g *Gateway) ensureReadyFromZero(ctx context.Context, fn *types.FunctionMetadata) error {
	if g.scale == nil || g.scale.gates == nil {
		if err := g.scaleFromZero(ctx, fn); err != nil {
			return err
		}
		return g.waitForFunctionReady(ctx, fn.Name, 30*time.Second)
	}

	var lastErr error
	for attempt := range coldStartMaxAttempts {
		cs, err := g.scale.gates.AcquireColdStartCtx(ctx, fn.Name)
		if err != nil {
			// Cancelled while parked behind a reclaim.
			return err
		}

		if cs.Leader {
			start := time.Now()
			err := g.runLeaderColdStart(ctx, fn, cs.Complete)
			if err == nil {
				metrics.RecordColdStart(fn.Name, "success", time.Since(start).Seconds())
				return nil
			}
			metrics.RecordColdStart(fn.Name, "error", time.Since(start).Seconds())
			if ctx.Err() != nil {
				return err
			}
			lastErr = err
			continue
		}

		// Follower or already-ready path.
		if err := cs.Wait(ctx); err != nil {
			if ctx.Err() != nil {
				// Our own deadline/cancel: report it, do not retry.
				return err
			}
			// The LEADER failed while we are still live: retry; the gate is back
			// at zero so the re-acquire elects a fresh leader.
			lastErr = err
			continue
		}

		// The gate reports ready. Verify a replica actually runs to catch
		// readiness left stale by a container that died outside a reclaim.
		genAtReady := g.scale.gates.Generation(fn.Name)
		if g.isContainerHealthy(ctx, fn.Name) {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		g.scale.gates.ReportZeroObserved(fn.Name, genAtReady)
		lastErr = fmt.Errorf("function %s: gate ready but no running container observed (stale readiness demoted; attempt %d)", fn.Name, attempt+1)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("function %s: cold start did not converge in %d attempts", fn.Name, coldStartMaxAttempts)
	}
	return lastErr
}

// runLeaderColdStart executes the leader's scale-from-zero and GUARANTEES that
// complete is called exactly once, even if the provider call panics (RT-215).
// A skipped Complete would leave g.cold set forever: every later invocation of
// the function would park on a channel that never closes, and both reclaim and
// crash-demotion refuse to run while a cold start is marked in-progress — a
// permanent per-function outage. A panic is converted into an error result so
// followers unblock and the state machine returns to zero.
func (g *Gateway) runLeaderColdStart(ctx context.Context, fn *types.FunctionMetadata, complete func(error)) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("cold start panicked for %s: %v", fn.Name, rec)
			g.logger.Errorf("%v", err)
		}
		complete(err)
	}()
	err = g.leaderColdStart(ctx, fn)
	return err
}

func (g *Gateway) leaderColdStart(ctx context.Context, fn *types.FunctionMetadata) error {
	if err := g.scaleFromZero(ctx, fn); err != nil {
		return err
	}
	return g.waitForFunctionReady(ctx, fn.Name, 30*time.Second)
}
