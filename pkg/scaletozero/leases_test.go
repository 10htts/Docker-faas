package scaletozero

import (
	"testing"
	"time"

	"github.com/docker-faas/docker-faas/pkg/faascontract"
)

func TestLeaseHonoredWhileFresh(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	reg := NewLeaseRegistry(clock)

	reg.Apply(faascontract.ActivityLeaseRequest{
		ContractVersion: faascontract.ContractVersion,
		Function:        "f",
		Generation:      3,
		Admitted:        1,
		Queued:          2,
		Running:         1,
		LeaseTTLSeconds: 30,
	})

	view := reg.View("f", 3)
	if !view.Present {
		t.Fatalf("expected lease present")
	}
	if view.DurableInFlight != 4 {
		t.Fatalf("DurableInFlight = %d, want 4", view.DurableInFlight)
	}
	if view.Expired || view.StaleGeneration {
		t.Fatalf("fresh matching lease must not be expired/stale")
	}
}

// TestExpiredLeaseDropsCountsToZero is the SZ-08 fail-safe: a lost renewal must
// not pin a dead function warm forever. The last-activity FACT survives expiry
// (RT-203): the idle window is measured from the final renewal, not from the
// expiry instant, which delays reclaim by at most one idle window and can never
// pin a function indefinitely.
func TestExpiredLeaseDropsCountsToZero(t *testing.T) {
	base := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	current := base
	reg := NewLeaseRegistry(func() time.Time { return current })

	reg.Apply(faascontract.ActivityLeaseRequest{
		ContractVersion: faascontract.ContractVersion,
		Function:        "f",
		Generation:      1,
		Running:         5,
		LeaseTTLSeconds: 10,
	})

	// Advance past the TTL.
	current = base.Add(11 * time.Second)

	view := reg.View("f", 1)
	if !view.Present || !view.Expired {
		t.Fatalf("expected expired lease present, got %+v", view)
	}
	if view.DurableInFlight != 0 {
		t.Fatalf("expired lease must not contribute in-flight work, got %d", view.DurableInFlight)
	}
	if !view.LastActivity.Equal(base) {
		t.Fatalf("expired lease must still report the last durable activity (got %v, want %v)", view.LastActivity, base)
	}
}

// TestStaleGenerationLeaseIgnored is the generation fence at the lease level: a
// lease sampled under an older generation cannot hold the current container.
func TestStaleGenerationLeaseIgnored(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	reg := NewLeaseRegistry(func() time.Time { return now })

	reg.Apply(faascontract.ActivityLeaseRequest{
		ContractVersion: faascontract.ContractVersion,
		Function:        "f",
		Generation:      5,
		Running:         9,
		LeaseTTLSeconds: 60,
	})

	// The live generation has moved on (e.g. a cold start bumped it to 7).
	view := reg.View("f", 7)
	if !view.StaleGeneration {
		t.Fatalf("expected stale generation flag")
	}
	if view.DurableInFlight != 0 {
		t.Fatalf("stale-generation lease must not contribute in-flight work, got %d", view.DurableInFlight)
	}
}

func TestDefaultLeaseTTLApplied(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	reg := NewLeaseRegistry(func() time.Time { return now })
	expires := reg.Apply(faascontract.ActivityLeaseRequest{
		ContractVersion: faascontract.ContractVersion,
		Function:        "f",
		Generation:      1,
		LeaseTTLSeconds: 0, // unset -> default
	})
	if !expires.Equal(now.Add(DefaultLeaseTTL)) {
		t.Fatalf("expires = %v, want %v", expires, now.Add(DefaultLeaseTTL))
	}
}
