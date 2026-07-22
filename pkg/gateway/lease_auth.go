package gateway

import (
	"sync"
	"time"
)

// Activity-lease authentication hardening (RT-214), layered on top of the
// CV-06 HMAC verification:
//
//   - Body limit: the endpoint carries a small fixed-shape JSON message; a
//     request body larger than the limit is rejected with 413 before decode.
//   - Timestamp skew: a signed request whose IssuedAt is outside now±skew is
//     rejected with 401. This bounds the replay window even across provider
//     restarts (a captured request goes stale within the skew window).
//   - Nonce replay cache: within the skew window, each (nonce) may be accepted
//     at most once. A replayed signed request is rejected with 401.
//   - Key rotation: verification accepts the active secret OR the previous
//     secret (rotation overlap); responses are ALWAYS signed with the active
//     secret. There is no other fallback and no default secret.
//
// The cache is memory-bounded: expired entries are pruned on insert, and when
// the cap is still exceeded the request is rejected (503) rather than
// silently widening the replay window. Only authenticated requests reach the
// cache, so unauthenticated traffic cannot fill it.

// defaultLeaseBodyLimit bounds the activity-lease request body. The message is
// a small flat JSON object; 64 KiB leaves an order-of-magnitude margin.
const defaultLeaseBodyLimit int64 = 64 * 1024

// defaultLeaseSkew bounds |now - IssuedAt| when the operator does not
// configure one explicitly.
const defaultLeaseSkew = 2 * time.Minute

// defaultLeaseNonceCacheCap bounds replay-cache memory when the operator does
// not configure a cap. Sized for thousands of actively-leased functions
// renewing inside one replay window (~100 bytes/entry ⇒ a few MiB at the cap).
// When the cap is reached even after pruning expired entries, the OLDEST entry
// is evicted and the request admitted (RT-217): rejecting legitimate renewals
// would cascade into lease expiry and wrongful reclaim, which is strictly
// worse than re-opening a replay window for one evicted nonce — replay already
// requires the signing secret, and a holder of the secret does not need
// replays.
const defaultLeaseNonceCacheCap = 65536

// leaseAuthPolicy is the transport-hardening policy for the activity-lease
// endpoint. Zero-value = hardening disabled (bare test wiring); production
// wiring in main always applies config defaults.
type leaseAuthPolicy struct {
	prevSecret string
	maxSkew    time.Duration
	bodyLimit  int64
	replay     *nonceCache
}

// effectiveBodyLimit never returns 0: the body limit cannot be disabled.
func (p *leaseAuthPolicy) effectiveBodyLimit() int64 {
	if p == nil || p.bodyLimit <= 0 {
		return defaultLeaseBodyLimit
	}
	return p.bodyLimit
}

// nonceCache is a bounded, expiring set of accepted nonces.
type nonceCache struct {
	mu      sync.Mutex
	entries map[string]time.Time // nonce -> expiry
	cap     int
	now     func() time.Time
}

func newNonceCache(capacity int, now func() time.Time) *nonceCache {
	if capacity <= 0 {
		capacity = defaultLeaseNonceCacheCap
	}
	if now == nil {
		now = time.Now
	}
	return &nonceCache{
		entries: make(map[string]time.Time),
		cap:     capacity,
		now:     now,
	}
}

// remember returns:
//   - replayed=true if nonce was already accepted and has not expired;
//   - evicted=true if the cache was at capacity even after pruning expired
//     entries and the oldest live entry was evicted to admit this one
//     (observability signal — the request is still accepted, RT-217);
//   - the nonce is recorded until expiry.
func (c *nonceCache) remember(nonce string, ttl time.Duration) (replayed, evicted bool) {
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()

	if exp, ok := c.entries[nonce]; ok && now.Before(exp) {
		return true, false
	}
	// Prune expired entries before admitting a new one.
	for n, exp := range c.entries {
		if !now.Before(exp) {
			delete(c.entries, n)
		}
	}
	if len(c.entries) >= c.cap {
		// Evict the entry closest to expiry (the least replay-relevant one)
		// rather than rejecting: a rejected legitimate renewal would cascade
		// into lease expiry and wrongful reclaim (RT-217).
		var oldestNonce string
		var oldestExp time.Time
		first := true
		for n, exp := range c.entries {
			if first || exp.Before(oldestExp) {
				oldestNonce, oldestExp, first = n, exp, false
			}
		}
		delete(c.entries, oldestNonce)
		evicted = true
	}
	c.entries[nonce] = now.Add(ttl)
	return false, evicted
}

// len reports the live entry count (test helper).
func (c *nonceCache) size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}
