package gateway

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

type BuildEntry struct {
	ID         string    `json:"id"`
	Name       string    `json:"name,omitempty"`
	Image      string    `json:"image,omitempty"`
	SourceType string    `json:"sourceType,omitempty"`
	Runtime    string    `json:"runtime,omitempty"`
	GitURL     string    `json:"gitUrl,omitempty"`
	GitRef     string    `json:"gitRef,omitempty"`
	SourcePath string    `json:"sourcePath,omitempty"`
	ZipName    string    `json:"zipName,omitempty"`
	Status     string    `json:"status"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
	DurationMs int64     `json:"durationMs,omitempty"`
	Deployed   bool      `json:"deployed,omitempty"`
	Updated    bool      `json:"updated,omitempty"`
	Output     string    `json:"output,omitempty"`
	Truncated  bool      `json:"truncated,omitempty"`
	Error      string    `json:"error,omitempty"`
}

type BuildUpdate struct {
	Name       *string
	Image      *string
	SourceType *string
	Runtime    *string
	GitURL     *string
	GitRef     *string
	SourcePath *string
	ZipName    *string
	Status     *string
	FinishedAt *time.Time
	DurationMs *int64
	Deployed   *bool
	Updated    *bool
	Output     *string
	Truncated  *bool
	Error      *string
}

type BuildTracker struct {
	mu        sync.RWMutex
	entries   []BuildEntry
	index     map[string]int
	limit     int
	retention time.Duration
	subs      map[int]chan BuildEntry
	nextSubID int
}

func NewBuildTracker(limit int, retention time.Duration) *BuildTracker {
	if limit <= 0 {
		limit = 100
	}
	return &BuildTracker{
		entries:   make([]BuildEntry, 0, limit),
		index:     make(map[string]int),
		limit:     limit,
		retention: retention,
		subs:      make(map[int]chan BuildEntry),
	}
}

func (t *BuildTracker) Add(entry BuildEntry) BuildEntry {
	t.mu.Lock()
	defer t.mu.Unlock()

	if entry.ID == "" {
		entry.ID = generateBuildID()
	}

	t.pruneLocked()
	t.entries = append([]BuildEntry{entry}, t.entries...)
	t.rebuildIndexLocked()
	t.trimLocked()
	t.broadcastLocked(entry)
	return entry
}

func (t *BuildTracker) Update(id string, update BuildUpdate) (BuildEntry, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	idx, ok := t.index[id]
	if !ok {
		return BuildEntry{}, false
	}
	entry := t.entries[idx]
	if update.Name != nil {
		entry.Name = *update.Name
	}
	if update.Image != nil {
		entry.Image = *update.Image
	}
	if update.SourceType != nil {
		entry.SourceType = *update.SourceType
	}
	if update.Runtime != nil {
		entry.Runtime = *update.Runtime
	}
	if update.GitURL != nil {
		entry.GitURL = *update.GitURL
	}
	if update.GitRef != nil {
		entry.GitRef = *update.GitRef
	}
	if update.SourcePath != nil {
		entry.SourcePath = *update.SourcePath
	}
	if update.ZipName != nil {
		entry.ZipName = *update.ZipName
	}
	if update.Status != nil {
		entry.Status = *update.Status
	}
	if update.FinishedAt != nil {
		entry.FinishedAt = *update.FinishedAt
	}
	if update.DurationMs != nil {
		entry.DurationMs = *update.DurationMs
	}
	if update.Deployed != nil {
		entry.Deployed = *update.Deployed
	}
	if update.Updated != nil {
		entry.Updated = *update.Updated
	}
	if update.Output != nil {
		entry.Output = *update.Output
	}
	if update.Truncated != nil {
		entry.Truncated = *update.Truncated
	}
	if update.Error != nil {
		entry.Error = *update.Error
	}

	t.entries[idx] = entry
	t.pruneLocked()
	idx, ok = t.index[id]
	if !ok {
		return BuildEntry{}, false
	}
	entry = t.entries[idx]
	t.broadcastLocked(entry)
	return entry, true
}

func (t *BuildTracker) List() []BuildEntry {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.pruneLocked()
	out := make([]BuildEntry, len(t.entries))
	copy(out, t.entries)
	return out
}

func (t *BuildTracker) Get(id string) (BuildEntry, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	idx, ok := t.index[id]
	if !ok {
		return BuildEntry{}, false
	}
	t.pruneLocked()
	idx, ok = t.index[id]
	if !ok {
		return BuildEntry{}, false
	}
	return t.entries[idx], true
}

func (t *BuildTracker) Clear() {
	t.mu.Lock()
	t.entries = []BuildEntry{}
	t.index = make(map[string]int)
	t.mu.Unlock()
}

func (t *BuildTracker) Subscribe() (<-chan BuildEntry, func()) {
	t.mu.Lock()
	defer t.mu.Unlock()

	ch := make(chan BuildEntry, 10)
	id := t.nextSubID
	t.nextSubID++
	t.subs[id] = ch

	cancel := func() {
		t.mu.Lock()
		if existing, ok := t.subs[id]; ok {
			delete(t.subs, id)
			close(existing)
		}
		t.mu.Unlock()
	}

	return ch, cancel
}

func (t *BuildTracker) rebuildIndexLocked() {
	t.index = make(map[string]int, len(t.entries))
	for i, entry := range t.entries {
		t.index[entry.ID] = i
	}
}

func (t *BuildTracker) trimLocked() {
	if len(t.entries) <= t.limit {
		return
	}
	t.entries = t.entries[:t.limit]
	t.rebuildIndexLocked()
}

func (t *BuildTracker) pruneLocked() {
	if t.retention <= 0 || len(t.entries) == 0 {
		return
	}
	cutoff := time.Now().Add(-t.retention)
	filtered := t.entries[:0]
	for _, entry := range t.entries {
		ts := entry.FinishedAt
		if ts.IsZero() {
			ts = entry.StartedAt
		}
		if ts.After(cutoff) {
			filtered = append(filtered, entry)
		}
	}
	t.entries = filtered
	t.rebuildIndexLocked()
}

func (t *BuildTracker) broadcastLocked(entry BuildEntry) {
	for _, ch := range t.subs {
		select {
		case ch <- entry:
		default:
		}
	}
}

func generateBuildID() string {
	return generateCallID()
}

func writeBuildStream(w http.ResponseWriter, r *http.Request, tracker *BuildTracker) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, cancel := tracker.Subscribe()
	defer cancel()

	for {
		select {
		case entry := <-ch:
			data, err := json.Marshal(entry)
			if err != nil {
				continue
			}
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(data)
			_, _ = w.Write([]byte("\n\n"))
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
