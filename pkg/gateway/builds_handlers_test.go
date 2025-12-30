package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandleListBuildsFilters(t *testing.T) {
	tracker := NewBuildTracker(10, 0)
	now := time.Now().UTC()
	tracker.Add(BuildEntry{
		ID:         "one",
		Name:       "alpha",
		SourceType: "git",
		Status:     "success",
		StartedAt:  now.Add(-1 * time.Hour),
		Output:     "build ok",
	})
	tracker.Add(BuildEntry{
		ID:         "two",
		Name:       "beta",
		SourceType: "zip",
		Status:     "failed",
		StartedAt:  now,
		Output:     "build failed",
	})

	gw := newTestGateway(&fakeStore{}, &fakeProvider{}, &fakeRouter{})
	gw.SetBuildTracker(tracker)

	req := httptest.NewRequest("GET", "/system/builds?status=failed&sourceType=zip&includeOutput=false", nil)
	rr := httptest.NewRecorder()

	gw.HandleListBuilds(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var entries []BuildEntry
	if err := json.Unmarshal(rr.Body.Bytes(), &entries); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].ID != "two" {
		t.Fatalf("expected entry two, got %s", entries[0].ID)
	}
	if entries[0].Output != "" {
		t.Fatalf("expected output to be stripped")
	}
}

func TestHandleListBuildsInvalidSince(t *testing.T) {
	tracker := NewBuildTracker(10, 0)
	tracker.Add(BuildEntry{
		ID:        "one",
		Status:    "success",
		StartedAt: time.Now().UTC(),
	})

	gw := newTestGateway(&fakeStore{}, &fakeProvider{}, &fakeRouter{})
	gw.SetBuildTracker(tracker)

	req := httptest.NewRequest("GET", "/system/builds?since=not-a-time", nil)
	rr := httptest.NewRecorder()

	gw.HandleListBuilds(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}
