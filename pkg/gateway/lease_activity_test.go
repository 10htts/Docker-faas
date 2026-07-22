package gateway

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/docker-faas/docker-faas/pkg/faascontract"
	"github.com/docker-faas/docker-faas/pkg/types"
)

// TestAcceptedLeaseFoldsActivityIntoGate proves the fix for the cross-function
// prune hole: an accepted activity-lease renewal must immediately fold its
// activity into the gate's authoritative anchor (gates.MarkActivity), not wait
// for the next reconcile pass. Otherwise another function's Apply could prune
// this function's expired lease before the fold, losing the anchor and allowing
// a premature reclaim.
func TestAcceptedLeaseFoldsActivityIntoGate(t *testing.T) {
	store := &syncStore{functions: map[string]*types.FunctionMetadata{
		"report": {Name: "report", Image: "img", Network: "net"},
	}}
	provider := &coldStartProvider{}
	gw, gates, _ := newScaleTestGateway(store, provider)

	if !gates.LastActivity("report").IsZero() {
		t.Fatalf("precondition: gate activity must start zero")
	}

	req := faascontract.ActivityLeaseRequest{
		ContractVersion: faascontract.ContractVersion,
		Function:        "report",
		Generation:      0,
		Admitted:        2,
		LeaseTTLSeconds: 30,
		IssuedAt:        time.Now(),
		Nonce:           "lease-activity-1",
	}
	body := signedLeaseBody(t, req)

	rr := httptest.NewRecorder()
	gw.HandleActivityLease(rr, httptest.NewRequest(http.MethodPost, "/system/scale/activity-lease", bytes.NewReader(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("accepted lease expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	if gates.LastActivity("report").IsZero() {
		t.Fatal("accepted lease must fold activity into the gate anchor immediately")
	}
}

// TestStaleGenerationLeaseDoesNotFoldActivity: a stale-generation lease is not
// "accepted" and must NOT bump the gate anchor (it references a container fenced
// out by a newer cold start).
func TestStaleGenerationLeaseDoesNotFoldActivity(t *testing.T) {
	store := &syncStore{functions: map[string]*types.FunctionMetadata{
		"report": {Name: "report", Image: "img", Network: "net"},
	}}
	provider := &coldStartProvider{}
	gw, gates, _ := newScaleTestGateway(store, provider)

	// Advance the live generation past 0 so a gen-0 lease is stale.
	cs := gates.AcquireColdStart("report")
	cs.Complete(nil)
	// Reclaim to clear ready so nothing else interferes; generation stays >0.
	if gates.TryBeginReclaim("report", gates.Generation("report")) {
		gates.FinishReclaim("report", true)
	}
	liveGen := gates.Generation("report")
	if liveGen == 0 {
		t.Fatalf("precondition: live generation must be > 0")
	}
	// Record the anchor the cold start set, then post a STALE (gen 0) lease.
	before := gates.LastActivity("report")

	req := faascontract.ActivityLeaseRequest{
		ContractVersion: faascontract.ContractVersion,
		Function:        "report",
		Generation:      0, // stale
		Admitted:        5,
		LeaseTTLSeconds: 30,
		IssuedAt:        time.Now(),
		Nonce:           "lease-stale-1",
	}
	rr := httptest.NewRecorder()
	gw.HandleActivityLease(rr, httptest.NewRequest(http.MethodPost, "/system/scale/activity-lease", bytes.NewReader(signedLeaseBody(t, req))))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 (accepted=false in body), got %d", rr.Code)
	}
	// A stale lease must not push the anchor forward.
	if gates.LastActivity("report").After(before) {
		t.Fatal("stale-generation lease must not fold activity into the gate")
	}
}
