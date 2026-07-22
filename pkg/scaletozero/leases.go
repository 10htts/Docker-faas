package scaletozero

import (
	"sync"
	"time"

	"github.com/docker-faas/docker-faas/pkg/faascontract"
)

// DefaultLeaseTTL is used when a lease request does not specify one.
const DefaultLeaseTTL = 30 * time.Second

// LeaseRegistry stores the most recent durable-work lease per function received
// from the AIDrivenMES control plane. It is the provider's memory of durable
// state it cannot itself observe (admitted/queued/running jobs), combined at
// decision time with the gateway's own in-flight HTTP accounting (SZ-12).
//
// Expiry is a fail-safe: an expired lease's counts are dropped to zero so a
// lost lease renewal cannot pin a dead function warm forever (SZ-08). A lease
// whose generation is older than the live generation is likewise ignored so a
// stale lease cannot influence the decision.
type LeaseRegistry struct {
	mu     sync.Mutex
	leases map[string]storedLease
	now    func() time.Time
}

type storedLease struct {
	admitted   int
	queued     int
	running    int
	generation uint64
	expiresAt  time.Time
	lastActive time.Time
}

// NewLeaseRegistry creates an empty registry. Pass nil clock for time.Now.
func NewLeaseRegistry(clock func() time.Time) *LeaseRegistry {
	if clock == nil {
		clock = time.Now
	}
	return &LeaseRegistry{
		leases: make(map[string]storedLease),
		now:    clock,
	}
}

// Apply records a validated lease request and returns when the honored lease
// expires. The request must already have passed Validate.
func (r *LeaseRegistry) Apply(req faascontract.ActivityLeaseRequest) time.Time {
	ttl := time.Duration(req.LeaseTTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = DefaultLeaseTTL
	}
	now := r.now()
	expires := now.Add(ttl)

	// A lease renewal is an assertion of activity as of the moment it is
	// received; the control plane keeps a function warm by renewing and lets it
	// go idle by ceasing to renew. lastActive is therefore anchored at now and
	// never at a caller-supplied future timestamp, so a buggy or hostile
	// (secret-holding) peer cannot pin a function warm indefinitely by claiming
	// activity in the future (RT-221). Equivalent to clamping the previous
	// latest(req.LastActivityAt, now) to now, which for every real (past)
	// timestamp already returned now.
	lastActive := now

	r.mu.Lock()
	// Opportunistic prune (RT-220): drop expired leases while we hold the lock so
	// the map cannot grow without bound over function churn when the idle
	// reconciler (the other pruner) is disabled. Bounded work: only entries whose
	// TTL has already passed are removed, and the current function is rewritten
	// below regardless.
	for fn, l := range r.leases {
		if !now.Before(l.expiresAt) {
			delete(r.leases, fn)
		}
	}
	r.leases[req.Function] = storedLease{
		admitted:   req.Admitted,
		queued:     req.Queued,
		running:    req.Running,
		generation: req.Generation,
		expiresAt:  expires,
		lastActive: lastActive,
	}
	r.mu.Unlock()
	return expires
}

// LeaseView is the expiry- and generation-resolved durable state for a
// function at a point in time.
type LeaseView struct {
	// DurableInFlight is admitted+queued+running, or zero if the lease is
	// expired or references a stale generation.
	DurableInFlight int
	// LastActivity is the durable last-activity timestamp. It survives lease
	// EXPIRY (a fact about the past stays true) but is zeroed for a
	// stale-generation lease (sampled against a container that no longer
	// exists).
	LastActivity time.Time
	// Present is true if any lease is stored (even if expired).
	Present bool
	// Expired is true if a stored lease has passed its TTL.
	Expired bool
	// ExpiresAt is the stored lease expiry, if present.
	ExpiresAt time.Time
	// StaleGeneration is true if a stored lease references an older
	// generation than the live one.
	StaleGeneration bool
}

// View resolves the durable state for fn against the live generation.
func (r *LeaseRegistry) View(fn string, liveGeneration uint64) LeaseView {
	r.mu.Lock()
	l, ok := r.leases[fn]
	r.mu.Unlock()
	if !ok {
		return LeaseView{}
	}

	now := r.now()
	view := LeaseView{Present: true, ExpiresAt: l.expiresAt}

	if !now.Before(l.expiresAt) {
		// Expired: the COUNTS are not honored (fail-safe against a lost
		// renewal, SZ-08 — stale counts could pin a dead function warm
		// forever), but the last-activity timestamp still is. It is a fact
		// about the past, not a claim of ongoing work, and honoring it is
		// bounded: it can delay reclaim by at most one idle window past the
		// final renewal. Dropping it would let the pass right after expiry
		// reclaim a function whose control plane reported activity only
		// seconds earlier (RT-203: a renewal that lands between reconcile
		// passes and expires before the next pass would otherwise vanish).
		view.Expired = true
		view.LastActivity = l.lastActive
		return view
	}
	if l.generation < liveGeneration {
		// Stale generation: a lease sampled under an older fence cannot hold
		// the current container.
		view.StaleGeneration = true
		return view
	}

	view.DurableInFlight = l.admitted + l.queued + l.running
	view.LastActivity = l.lastActive
	return view
}

// Forget drops any stored lease for fn.
func (r *LeaseRegistry) Forget(fn string) {
	r.mu.Lock()
	delete(r.leases, fn)
	r.mu.Unlock()
}

func latest(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
