package gateway

import (
	"testing"
	"time"
)

func TestBuildTrackerAddUpdate(t *testing.T) {
	tracker := NewBuildTracker(10, 0)

	entry := BuildEntry{
		Name:       "hello",
		SourceType: "zip",
		Status:     "running",
	}
	entry = tracker.Add(entry)
	if entry.ID == "" {
		t.Fatal("expected build ID")
	}

	status := "success"
	image := "docker-faas/hello:latest"
	updatedEntry, ok := tracker.Update(entry.ID, BuildUpdate{
		Status: &status,
		Image:  &image,
	})
	if !ok {
		t.Fatal("expected update to succeed")
	}
	if updatedEntry.Status != "success" {
		t.Fatalf("expected status success, got %s", updatedEntry.Status)
	}
	if updatedEntry.Image != image {
		t.Fatalf("expected image %s, got %s", image, updatedEntry.Image)
	}
}

func TestBuildTrackerRetention(t *testing.T) {
	tracker := NewBuildTracker(10, time.Minute)

	oldTime := time.Now().Add(-2 * time.Minute)
	oldEntry := BuildEntry{
		ID:         "old",
		Name:       "old-build",
		Status:     "success",
		StartedAt:  oldTime,
		FinishedAt: oldTime,
	}
	tracker.Add(oldEntry)

	newEntry := BuildEntry{
		ID:        "new",
		Name:      "new-build",
		Status:    "success",
		StartedAt: time.Now(),
	}
	tracker.Add(newEntry)

	entries := tracker.List()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after retention, got %d", len(entries))
	}
	if entries[0].ID != "new" {
		t.Fatalf("expected new entry to remain, got %s", entries[0].ID)
	}
}
