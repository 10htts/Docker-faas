package scaletozero

import (
	"testing"
	"time"
)

// TestDecideNonPositiveIdleDurationHolds is the RT-226 defense-in-depth guard:
// a policy with a non-positive IdleDuration must HOLD, never scale to zero — even
// when the function looks idle. Without the guard, `idleFor < IdleDuration` is
// false for every idleFor>=0, so the function would be reclaimed at idleFor==0,
// which combined with restart seeding (LastActivity=now) would reap a function
// on the very pass it is seeded.
func TestDecideNonPositiveIdleDurationHolds(t *testing.T) {
	d := IdleDecider{}
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)

	for _, idle := range []time.Duration{0, -time.Second, -time.Hour} {
		p := Policy{Enabled: true, IdleDuration: idle}
		// Idle for a long time, no work in flight — the tempting reclaim case.
		snap := ActivitySnapshot{LastActivity: now.Add(-24 * time.Hour), ObservedReplicas: 1}
		got := d.Decide(now, p, snap)
		if got.Action != ActionHold {
			t.Fatalf("IdleDuration=%s must Hold, got %v (%s)", idle, got.Action, got.Reason)
		}
	}

	// Sanity: a positive window with the same idle history DOES scale to zero,
	// so the guard is not masking a broken decider.
	p := Policy{Enabled: true, IdleDuration: time.Minute}
	snap := ActivitySnapshot{LastActivity: now.Add(-24 * time.Hour), ObservedReplicas: 1}
	if got := d.Decide(now, p, snap); got.Action != ActionScaleToZero {
		t.Fatalf("positive idle window with old activity must ScaleToZero, got %v", got.Action)
	}
}
