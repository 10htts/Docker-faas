// Package scaletozero implements the provider (Docker-faas) half of idle
// function scale-to-zero: authoritative in-flight accounting, a
// generation/lease fence, a per-function idle decision, and one provider-owned
// reconcile operation to the desired replica count.
//
// The design answers redteam objections SZ-01, SZ-02, SZ-03, SZ-07, SZ-08 and
// SZ-09. It deliberately depends only on the standard library, the neutral
// faascontract schema, logrus, and the metrics recorders it is given, so it can
// be unit tested with a fake replica controller and never needs a real Docker
// daemon for the load-bearing safety tests.
package scaletozero

import (
	"fmt"
	"time"

	"github.com/docker-faas/docker-faas/pkg/faascontract"
)

// Policy is the per-function idle policy. AIDrivenMES is the authoritative home
// of this policy (SZ-11); on the provider side it is derived from function
// labels/annotations at deploy time with config defaults, so desired state is
// reconstructable after a gateway restart (SZ-07).
type Policy struct {
	// Enabled turns idle scale-to-zero on for this function. When false the
	// function is never automatically reclaimed.
	Enabled bool
	// IdleDuration is how long a function must be free of in-flight and
	// recent activity before it is reclaimed (SZ-08 stable window).
	IdleDuration time.Duration
	// MinReplicas is the warm minimum. When > 0 the function is pinned warm
	// and never reclaimed to zero (SZ-04 latency-critical exception).
	MinReplicas int
	// MaxReplicas caps replicas (informational for the idle reconciler).
	MaxReplicas int
}

// EffectiveWarmMinimum is the warm minimum actually enforced: MinReplicas
// clamped to MaxReplicas when both are set and in conflict. Without the clamp a
// policy declaring MinReplicas > MaxReplicas would make keep-warm scale a
// function PAST its own replica cap.
func (p Policy) EffectiveWarmMinimum() int {
	if p.MinReplicas <= 0 {
		return 0
	}
	if p.MaxReplicas > 0 && p.MinReplicas > p.MaxReplicas {
		return p.MaxReplicas
	}
	return p.MinReplicas
}

// Action is the reconciler action chosen for a function.
type Action int

const (
	// ActionHold keeps the function as-is (work in flight or not idle yet).
	ActionHold Action = iota
	// ActionScaleToZero reclaims the function to zero replicas.
	ActionScaleToZero
	// ActionKeepWarm ensures the function stays at its warm minimum.
	ActionKeepWarm
)

func (a Action) String() string {
	switch a {
	case ActionScaleToZero:
		return "scale_to_zero"
	case ActionKeepWarm:
		return "keep_warm"
	default:
		return "hold"
	}
}

// AsContractDecision maps an Action to the wire decision returned to
// AIDrivenMES.
func (a Action) AsContractDecision() faascontract.Decision {
	switch a {
	case ActionScaleToZero:
		return faascontract.DecisionScaleToZero
	case ActionKeepWarm:
		return faascontract.DecisionKeepWarm
	default:
		return faascontract.DecisionHold
	}
}

// ActivitySnapshot is the combined, expiry-resolved view of all work for a
// function at a point in time. It is the authoritative in-flight accounting
// that the idle decision is made on — NOT merely the last-request timestamp
// (SZ-03).
type ActivitySnapshot struct {
	// GatewayInFlight is the provider-owned count of HTTP invocations
	// currently executing through the gateway (sync + async), including
	// long-running and cancelling work that has not returned.
	GatewayInFlight int
	// DurableInFlight is the honored durable count from AIDrivenMES
	// (admitted + queued + running). Counts from an EXPIRED lease are already
	// dropped to zero by the caller so a lost lease cannot pin a dead function
	// forever (SZ-08).
	DurableInFlight int
	// LastActivity is the most recent activity across gateway and durable
	// sources.
	LastActivity time.Time
	// ObservedReplicas is the provider-observed running replica count.
	ObservedReplicas int
}

// TotalInFlight is the authoritative total of work that must protect a function
// from reclamation.
func (s ActivitySnapshot) TotalInFlight() int {
	return s.GatewayInFlight + s.DurableInFlight
}

// Decision is the outcome of Decide.
type Decision struct {
	Action          Action
	Reason          string
	DesiredReplicas int
}

// IdleDecider makes the pure idle decision for a single function. It holds no
// state and never touches Docker, so its rules are exhaustively unit testable.
type IdleDecider struct{}

// Decide computes the idle action for a function given its policy, the combined
// activity snapshot, and the current time.
//
// Rules, in priority order:
//  1. Disabled policy -> Hold (never auto-reclaim).
//  2. Warm minimum (MinReplicas>0) -> KeepWarm; never scale to zero (SZ-04).
//  3. Any in-flight work (gateway HTTP or durable) -> Hold (SZ-01/SZ-03).
//  4. Idle window not elapsed since last activity -> Hold (SZ-08 stable window).
//  5. Otherwise -> ScaleToZero.
func (IdleDecider) Decide(now time.Time, p Policy, s ActivitySnapshot) Decision {
	if !p.Enabled {
		return Decision{Action: ActionHold, Reason: "idle scale-to-zero disabled for function", DesiredReplicas: s.ObservedReplicas}
	}

	if p.MinReplicas > 0 {
		effectiveMin := p.EffectiveWarmMinimum()
		reason := fmt.Sprintf("warm minimum pinned at %d replicas", effectiveMin)
		if effectiveMin != p.MinReplicas {
			reason = fmt.Sprintf("warm minimum %d clamped to max replicas %d", p.MinReplicas, p.MaxReplicas)
		}
		return Decision{
			Action:          ActionKeepWarm,
			Reason:          reason,
			DesiredReplicas: effectiveMin,
		}
	}

	if inflight := s.TotalInFlight(); inflight > 0 {
		return Decision{
			Action:          ActionHold,
			Reason:          fmt.Sprintf("work in flight: gateway=%d durable=%d", s.GatewayInFlight, s.DurableInFlight),
			DesiredReplicas: max(s.ObservedReplicas, 1),
		}
	}

	if p.IdleDuration <= 0 {
		// A non-positive idle window is a misconfiguration; treat it as "never
		// idle" and Hold rather than reclaim immediately (which, combined with
		// restart seeding that stamps LastActivity=now, would reap a function on
		// the very pass it is seeded). Policy sources already floor this to a sane
		// default; this keeps the safety invariant local to the decision (RT-226).
		return Decision{
			Action:          ActionHold,
			Reason:          "idle duration not positive; holding (misconfiguration)",
			DesiredReplicas: max(s.ObservedReplicas, 0),
		}
	}

	if s.LastActivity.IsZero() {
		// No activity history: we cannot prove the idle window has elapsed, so
		// hold conservatively rather than reap a possibly just-started function.
		return Decision{
			Action:          ActionHold,
			Reason:          "no recorded activity yet; holding until idle window can be measured",
			DesiredReplicas: max(s.ObservedReplicas, 0),
		}
	}

	idleFor := now.Sub(s.LastActivity)
	if idleFor < p.IdleDuration {
		remaining := p.IdleDuration - idleFor
		return Decision{
			Action:          ActionHold,
			Reason:          fmt.Sprintf("idle %s of required %s (%s remaining)", idleFor.Round(time.Second), p.IdleDuration, remaining.Round(time.Second)),
			DesiredReplicas: max(s.ObservedReplicas, 0),
		}
	}

	return Decision{
		Action:          ActionScaleToZero,
		Reason:          fmt.Sprintf("idle for at least %s with no work in flight", p.IdleDuration),
		DesiredReplicas: 0,
	}
}
