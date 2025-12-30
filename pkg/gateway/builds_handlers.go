package gateway

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

// HandleListBuilds handles GET /system/builds.
func (g *Gateway) HandleListBuilds(w http.ResponseWriter, r *http.Request) {
	if g.builds == nil {
		http.Error(w, "build tracking disabled", http.StatusNotFound)
		return
	}
	filter, err := parseBuildListFilter(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	entries := filterBuildEntries(g.builds.List(), filter)
	g.writeJSON(w, http.StatusOK, entries)
}

// HandleGetBuild handles GET /system/builds/{id}.
func (g *Gateway) HandleGetBuild(w http.ResponseWriter, r *http.Request) {
	if g.builds == nil {
		http.Error(w, "build tracking disabled", http.StatusNotFound)
		return
	}
	id := mux.Vars(r)["id"]
	entry, ok := g.builds.Get(id)
	if !ok {
		http.Error(w, "build not found", http.StatusNotFound)
		return
	}
	includeOutput, err := parseOptionalBool(r.URL.Query().Get("includeOutput"), true)
	if err != nil {
		http.Error(w, "invalid includeOutput value", http.StatusBadRequest)
		return
	}
	if !includeOutput {
		entry.Output = ""
		entry.Truncated = false
	}
	g.writeJSON(w, http.StatusOK, entry)
}

// HandleClearBuilds handles DELETE /system/builds.
func (g *Gateway) HandleClearBuilds(w http.ResponseWriter, r *http.Request) {
	if g.builds == nil {
		http.Error(w, "build tracking disabled", http.StatusNotFound)
		return
	}
	g.builds.Clear()
	w.WriteHeader(http.StatusNoContent)
}

// HandleBuildStream handles GET /system/builds/stream.
func (g *Gateway) HandleBuildStream(w http.ResponseWriter, r *http.Request) {
	if g.builds == nil {
		http.Error(w, "build tracking disabled", http.StatusNotFound)
		return
	}
	writeBuildStream(w, r, g.builds)
}

type buildListFilter struct {
	statuses      map[string]struct{}
	sourceTypes   map[string]struct{}
	nameContains  string
	since         time.Time
	before        time.Time
	limit         int
	includeOutput bool
}

func parseBuildListFilter(r *http.Request) (buildListFilter, error) {
	query := r.URL.Query()
	filter := buildListFilter{
		includeOutput: true,
	}

	if raw := strings.TrimSpace(query.Get("name")); raw != "" {
		filter.nameContains = strings.ToLower(raw)
	}
	if raw := strings.TrimSpace(query.Get("status")); raw != "" {
		filter.statuses = parseCSVSet(raw)
	}
	if raw := strings.TrimSpace(query.Get("sourceType")); raw != "" {
		filter.sourceTypes = parseCSVSet(raw)
	}
	if raw := strings.TrimSpace(query.Get("since")); raw != "" {
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return filter, err
		}
		filter.since = parsed
	}
	if raw := strings.TrimSpace(query.Get("before")); raw != "" {
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return filter, err
		}
		filter.before = parsed
	}
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return filter, err
		}
		if parsed < 0 {
			return filter, fmt.Errorf("limit must be >= 0")
		}
		filter.limit = parsed
	}
	if raw := strings.TrimSpace(query.Get("includeOutput")); raw != "" {
		parsed, err := parseOptionalBool(raw, true)
		if err != nil {
			return filter, err
		}
		filter.includeOutput = parsed
	}

	return filter, nil
}

func parseCSVSet(value string) map[string]struct{} {
	parts := strings.Split(value, ",")
	set := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		trimmed := strings.ToLower(strings.TrimSpace(part))
		if trimmed == "" {
			continue
		}
		set[trimmed] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

func parseOptionalBool(value string, defaultValue bool) (bool, error) {
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue, err
	}
	return parsed, nil
}

func filterBuildEntries(entries []BuildEntry, filter buildListFilter) []BuildEntry {
	filtered := make([]BuildEntry, 0, len(entries))
	for _, entry := range entries {
		if filter.nameContains != "" && !strings.Contains(strings.ToLower(entry.Name), filter.nameContains) {
			continue
		}
		if filter.statuses != nil {
			if _, ok := filter.statuses[strings.ToLower(entry.Status)]; !ok {
				continue
			}
		}
		if filter.sourceTypes != nil {
			if _, ok := filter.sourceTypes[strings.ToLower(entry.SourceType)]; !ok {
				continue
			}
		}
		if !filter.since.IsZero() && entry.StartedAt.Before(filter.since) {
			continue
		}
		if !filter.before.IsZero() && entry.StartedAt.After(filter.before) {
			continue
		}
		filtered = append(filtered, entry)
	}

	if filter.limit > 0 && len(filtered) > filter.limit {
		filtered = filtered[:filter.limit]
	}

	if !filter.includeOutput {
		for i := range filtered {
			filtered[i].Output = ""
			filtered[i].Truncated = false
		}
	}

	return filtered
}
