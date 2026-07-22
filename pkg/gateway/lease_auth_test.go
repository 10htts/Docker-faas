package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/docker-faas/docker-faas/pkg/faascontract"
	"github.com/docker-faas/docker-faas/pkg/types"
)

// RT-214 negative-path tests: timestamp skew, nonce replay, body limit, and
// secret rotation on POST /system/scale/activity-lease. They reuse the CV-06
// harness (newScaleTestGatewayWithSecret / signedLeaseBody) from
// scale_handlers_test.go. Enforcement is opt-in via SetLeaseAuthPolicy exactly
// as main wires it from config.

func newHardenedLeaseGateway(t *testing.T, skew time.Duration, bodyLimit int64, prevSecret string) *Gateway {
	t.Helper()
	store := &syncStore{functions: map[string]*types.FunctionMetadata{}}
	provider := &coldStartProvider{}
	gw, _, _ := newScaleTestGateway(store, provider)
	gw.SetLeaseAuthPolicy(prevSecret, skew, bodyLimit, 0)
	return gw
}

func baseLeaseRequest(nonce string, issuedAt time.Time) faascontract.ActivityLeaseRequest {
	return faascontract.ActivityLeaseRequest{
		ContractVersion: faascontract.ContractVersion,
		Function:        "fn",
		Generation:      0,
		Admitted:        1,
		LeaseTTLSeconds: 30,
		IssuedAt:        issuedAt,
		Nonce:           nonce,
	}
}

func postLease(t *testing.T, gw *Gateway, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/system/scale/activity-lease", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	gw.HandleActivityLease(rr, req)
	return rr
}

func TestActivityLeaseRejectsStaleTimestamp(t *testing.T) {
	gw := newHardenedLeaseGateway(t, 2*time.Minute, 0, "")
	body := signedLeaseBody(t, baseLeaseRequest("nonce-stale-1", time.Now().Add(-10*time.Minute)))
	rr := postLease(t, gw, body)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("stale issued_at must be rejected 401, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "issued_at") {
		t.Fatalf("rejection must name the issued_at window, got %q", rr.Body.String())
	}
}

func TestActivityLeaseRejectsFutureTimestamp(t *testing.T) {
	gw := newHardenedLeaseGateway(t, 2*time.Minute, 0, "")
	body := signedLeaseBody(t, baseLeaseRequest("nonce-future-1", time.Now().Add(10*time.Minute)))
	rr := postLease(t, gw, body)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("future issued_at must be rejected 401, got %d", rr.Code)
	}
}

func TestActivityLeaseRejectsMissingIssuedAtWhenSkewEnforced(t *testing.T) {
	gw := newHardenedLeaseGateway(t, 2*time.Minute, 0, "")
	body := signedLeaseBody(t, baseLeaseRequest("nonce-zero-ts", time.Time{}))
	rr := postLease(t, gw, body)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("zero issued_at must be rejected 401 when skew is enforced, got %d", rr.Code)
	}
}

func TestActivityLeaseSkewDisabledAllowsOldTimestamp(t *testing.T) {
	// skew=0 is the documented escape hatch: freshness unchecked, replay cache
	// still active with the built-in fallback window.
	gw := newHardenedLeaseGateway(t, 0, 0, "")
	body := signedLeaseBody(t, baseLeaseRequest("nonce-skew-off", time.Now().Add(-24*time.Hour)))
	rr := postLease(t, gw, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("with skew disabled an old issued_at must pass, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestActivityLeaseRejectsReplayedNonce(t *testing.T) {
	gw := newHardenedLeaseGateway(t, 2*time.Minute, 0, "")
	body := signedLeaseBody(t, baseLeaseRequest("nonce-replay-1", time.Now()))

	first := postLease(t, gw, body)
	if first.Code != http.StatusOK {
		t.Fatalf("first use of a nonce must succeed, got %d: %s", first.Code, first.Body.String())
	}
	second := postLease(t, gw, body)
	if second.Code != http.StatusUnauthorized {
		t.Fatalf("replayed nonce must be rejected 401, got %d: %s", second.Code, second.Body.String())
	}
	if !strings.Contains(second.Body.String(), "nonce") {
		t.Fatalf("replay rejection must name the nonce, got %q", second.Body.String())
	}
}

func TestActivityLeaseRejectsEmptyNonceWhenReplayEnforced(t *testing.T) {
	gw := newHardenedLeaseGateway(t, 2*time.Minute, 0, "")
	body := signedLeaseBody(t, baseLeaseRequest("", time.Now()))
	rr := postLease(t, gw, body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("empty nonce must be rejected 400 when replay protection is on, got %d", rr.Code)
	}
}

func TestActivityLeaseRejectsOversizedBodyBeforeAuth(t *testing.T) {
	gw := newHardenedLeaseGateway(t, 2*time.Minute, 256, "")
	// Deliberately unsigned garbage: the 413 must fire BEFORE signature
	// verification ever runs.
	big := append([]byte(`{"function":"`), bytes.Repeat([]byte("x"), 4096)...)
	big = append(big, []byte(`"}`)...)
	rr := postLease(t, gw, big)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body must be rejected 413 pre-auth, got %d", rr.Code)
	}
}

func TestActivityLeaseRotationAcceptsPreviousSignsWithActive(t *testing.T) {
	const previousSecret = "previous-rotation-secret"
	gw := newHardenedLeaseGateway(t, 2*time.Minute, 0, previousSecret)

	req := baseLeaseRequest("nonce-rotate-1", time.Now())
	body, err := faascontract.EncodeActivityLeaseRequest(req, previousSecret)
	if err != nil {
		t.Fatalf("encode with previous secret: %v", err)
	}
	rr := postLease(t, gw, body)
	if rr.Code != http.StatusOK {
		t.Fatalf("request signed with the previous secret must verify during rotation, got %d: %s", rr.Code, rr.Body.String())
	}

	// The response must be signed with the ACTIVE secret and echo our nonce.
	var resp faascontract.ActivityLeaseResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if err := faascontract.VerifyActivityLeaseResponse(resp, testLeaseSecret); err != nil {
		t.Fatalf("response must verify with the ACTIVE secret: %v", err)
	}
	if err := faascontract.VerifyActivityLeaseResponse(resp, previousSecret); err == nil {
		t.Fatalf("response must NOT be signed with the previous secret")
	}
	if resp.Nonce != "nonce-rotate-1" {
		t.Fatalf("response must echo the request nonce (binding), got %q", resp.Nonce)
	}
}

func TestActivityLeaseRotationStillRejectsUnknownSecret(t *testing.T) {
	gw := newHardenedLeaseGateway(t, 2*time.Minute, 0, "previous-rotation-secret")
	req := baseLeaseRequest("nonce-unknown-1", time.Now())
	body, err := faascontract.EncodeActivityLeaseRequest(req, "some-third-secret")
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	rr := postLease(t, gw, body)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("a third secret must never verify even during rotation, got %d", rr.Code)
	}
}

func TestNonceCacheBoundsAndExpiry(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	clock := func() time.Time { return now }
	c := newNonceCache(3, clock)

	// "a" gets the shortest TTL so it is deterministically the oldest entry.
	if replayed, evicted := c.remember("a", 30*time.Second); replayed || evicted {
		t.Fatalf("fresh nonce a must be admitted cleanly")
	}
	for _, n := range []string{"b", "c"} {
		if replayed, evicted := c.remember(n, time.Minute); replayed || evicted {
			t.Fatalf("fresh nonce %q must be admitted cleanly (replayed=%v evicted=%v)", n, replayed, evicted)
		}
	}
	if replayed, _ := c.remember("a", time.Minute); !replayed {
		t.Fatalf("duplicate nonce within TTL must report replayed")
	}

	// At capacity: the OLDEST live entry ("a") is evicted and the newcomer is
	// admitted (RT-217 — rejecting legitimate renewals would cascade into
	// wrongful reclaim).
	replayed, evicted := c.remember("d", time.Minute)
	if replayed || !evicted {
		t.Fatalf("cache at cap must evict oldest and admit (replayed=%v evicted=%v)", replayed, evicted)
	}
	if c.size() != 3 {
		t.Fatalf("size must stay at cap after eviction, got %d", c.size())
	}
	if replayed, _ := c.remember("b", time.Minute); !replayed {
		t.Fatalf("live entry b must have survived the eviction")
	}

	// Advance past expiry: pruning must free capacity, and an expired nonce is
	// admitted again — which is exactly why the replay TTL must exceed the
	// entire freshness window (a replay after TTL is already rejected as
	// stale by the skew check).
	now = now.Add(5 * time.Minute)
	if replayed, evicted := c.remember("e", time.Minute); replayed || evicted {
		t.Fatalf("expired entries must be pruned to admit new nonces (replayed=%v evicted=%v)", replayed, evicted)
	}
	if c.size() != 1 {
		t.Fatalf("expected only the fresh entry after pruning, got %d", c.size())
	}
}

func TestActivityLeaseInvalidRequestDoesNotBurnNonce(t *testing.T) {
	// A signed request that fails structural validation (negative count) must
	// be rejected 400 WITHOUT consuming its nonce, so the control plane can
	// retry a corrected request under the same nonce (RT-220).
	gw := newHardenedLeaseGateway(t, 2*time.Minute, 0, "")
	req := baseLeaseRequest("nonce-not-burned", time.Now())
	req.Admitted = -1
	rr := postLease(t, gw, signedLeaseBody(t, req))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("structurally invalid request must be 400, got %d: %s", rr.Code, rr.Body.String())
	}

	fixed := baseLeaseRequest("nonce-not-burned", time.Now())
	rr2 := postLease(t, gw, signedLeaseBody(t, fixed))
	if rr2.Code != http.StatusOK {
		t.Fatalf("corrected retry with the same nonce must succeed, got %d: %s", rr2.Code, rr2.Body.String())
	}
}

func TestActivityLeaseRejectsMalformedFunctionName(t *testing.T) {
	// Even a secret-holding peer cannot store leases under arbitrary byte
	// strings (metric-cardinality / storage hygiene, RT-220).
	gw := newHardenedLeaseGateway(t, 2*time.Minute, 0, "")
	req := baseLeaseRequest("nonce-badname", time.Now())
	req.Function = "Invalid_NAME!with spaces"
	rr := postLease(t, gw, signedLeaseBody(t, req))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("malformed function name must be 400, got %d: %s", rr.Code, rr.Body.String())
	}
}
