package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/docker-faas/docker-faas/pkg/provider"
)

// fallbackLogTail bounds the legacy one-shot log fallback when the caller did
// not specify a tail (the raw blob path cannot stream "everything" safely).
const fallbackLogTail = 100

// functionLogStreamer is the OPTIONAL provider capability used to stream
// function logs. *provider.DockerProvider implements it (see
// pkg/provider/logs_stream.go); the base Provider interface in interfaces.go
// stays untouched and providers without the capability fall back to the
// legacy GetContainerLogs blob.
type functionLogStreamer interface {
	StreamFunctionLogs(ctx context.Context, functionName string, since time.Time, tail int, follow bool) (<-chan provider.LogEntry, error)
}

// logMessage mirrors the pinned faas-provider log Message JSON exactly
// (github.com/openfaas/faas-provider v0.25.12 logs/logs.go): keys name,
// namespace, instance, timestamp, text — no omitempty, matching upstream.
type logMessage struct {
	Name      string    `json:"name"`
	Namespace string    `json:"namespace"`
	Instance  string    `json:"instance"`
	Timestamp time.Time `json:"timestamp"`
	Text      string    `json:"text"`
}

// logsQuery mirrors the pinned faas-provider logs.Request query contract:
// name (required), namespace, instance, since (RFC3339), tail (int),
// follow (bool).
type logsQuery struct {
	Name      string
	Namespace string
	Instance  string
	Since     time.Time
	Tail      int
	Follow    bool
}

// parseLogsQuery parses /system/logs query parameters. Any parse failure is a
// 422 per the pinned faas-provider logs handler.
func parseLogsQuery(r *http.Request) (logsQuery, error) {
	query := r.URL.Query()

	q := logsQuery{
		Name:      normalizeFunctionName(query.Get("name")),
		Namespace: query.Get("namespace"),
		Instance:  query.Get("instance"),
	}

	if q.Name == "" {
		return q, fmt.Errorf("name is required")
	}
	if err := validateFunctionName(q.Name); err != nil {
		return q, err
	}

	if tailStr := query.Get("tail"); tailStr != "" {
		tail, err := strconv.Atoi(tailStr)
		if err != nil {
			return q, fmt.Errorf("invalid tail value: %s", tailStr)
		}
		q.Tail = tail
	}

	if followStr := query.Get("follow"); followStr != "" {
		follow, err := strconv.ParseBool(followStr)
		if err != nil {
			return q, fmt.Errorf("invalid follow value: %s", followStr)
		}
		q.Follow = follow
	}

	if sinceStr := query.Get("since"); sinceStr != "" {
		since, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			return q, fmt.Errorf("invalid since value: %s", sinceStr)
		}
		q.Since = since
	}

	return q, nil
}

// HandleGetLogs handles GET /system/logs.
//
// Output is the pinned faas-provider log contract (v0.25.12 logs/handler.go):
// HTTP 200 with Content-Type application/x-ndjson, one JSON-encoded log
// Message per line, flushed after every message. Parse failures answer 422,
// unknown functions 404 (per the pinned gateway OpenAPI spec for
// /system/logs), unknown namespaces 400 like every other namespaced route
// here. faas-cli 0.18.0 `logs` decodes this stream with json.Decoder into
// faas-provider logs.Message.
func (g *Gateway) HandleGetLogs(w http.ResponseWriter, r *http.Request) {
	q, err := parseLogsQuery(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if err := validateNamespace(q.Namespace); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if _, err := g.store.GetFunction(q.Name); err != nil {
		http.Error(w, "Function not found", http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		// Pinned behavior: writers that cannot stream cannot serve logs.
		http.NotFound(w, r)
		return
	}

	entries, err := g.logEntries(r.Context(), q)
	if err != nil {
		g.logger.Errorf("Failed to get logs for %s: %v", q.Name, err)
		http.Error(w, fmt.Sprintf("Failed to get logs: %v", err), http.StatusInternalServerError)
		return
	}

	// Pinned response headers (faas-provider v0.25.12 logs/handler.go).
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Connection", "Keep-Alive")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	encoder := json.NewEncoder(w)
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case entry, ok := <-entries:
			if !ok {
				return
			}
			if q.Instance != "" && entry.Instance != q.Instance {
				continue
			}
			msg := logMessage{
				Name:      entry.Name,
				Namespace: DefaultFunctionNamespace,
				Instance:  entry.Instance,
				Timestamp: entry.Timestamp,
				Text:      entry.Text,
			}
			if err := encoder.Encode(msg); err != nil {
				g.logger.Debugf("Failed to write log message for %s: %v", q.Name, err)
				return
			}
			flusher.Flush()
		}
	}
}

// logEntries returns the log entry channel for a query: streamed from the
// provider when it implements the optional functionLogStreamer capability,
// otherwise a one-shot fallback built from the legacy GetContainerLogs blob.
func (g *Gateway) logEntries(ctx context.Context, q logsQuery) (<-chan provider.LogEntry, error) {
	if streamer, ok := g.provider.(functionLogStreamer); ok {
		return streamer.StreamFunctionLogs(ctx, q.Name, q.Since, q.Tail, q.Follow)
	}
	return g.fallbackLogEntries(ctx, q)
}

// fallbackLogEntries adapts the legacy blob API to the streaming contract:
// lines are split, docker frame headers stripped, and timestamps parsed
// best-effort (lines without one are stamped with the current time). follow
// is ignored — the blob is finite by construction.
func (g *Gateway) fallbackLogEntries(ctx context.Context, q logsQuery) (<-chan provider.LogEntry, error) {
	tail := q.Tail
	if tail <= 0 {
		tail = fallbackLogTail
	}

	blob, err := g.provider.GetContainerLogs(ctx, q.Name, tail)
	if err != nil {
		return nil, err
	}

	entries := parseLogBlob(q.Name, blob)
	out := make(chan provider.LogEntry, len(entries))
	for _, entry := range entries {
		out <- entry
	}
	close(out)
	return out, nil
}

// parseLogBlob converts a raw log blob into entries, one per non-empty line.
func parseLogBlob(functionName, blob string) []provider.LogEntry {
	lines := strings.Split(blob, "\n")
	entries := make([]provider.LogEntry, 0, len(lines))
	now := time.Now().UTC()
	for _, line := range lines {
		line = stripDockerFrameHeader(line)
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		ts, text := provider.ParseDockerLogLine(line)
		if ts.IsZero() {
			ts = now
		}
		entries = append(entries, provider.LogEntry{
			Name:      functionName,
			Instance:  functionName,
			Timestamp: ts,
			Text:      text,
		})
	}
	return entries
}

// stripDockerFrameHeader removes the 8-byte multiplexed stream frame header
// Docker prepends to each frame of non-TTY container output when the raw
// stream was read without stdcopy demultiplexing (the legacy GetContainerLogs
// path). Header layout: [STREAM_TYPE 0|1|2, 0, 0, 0, SIZE(4 bytes)].
func stripDockerFrameHeader(line string) string {
	if len(line) >= 8 &&
		(line[0] == 0x00 || line[0] == 0x01 || line[0] == 0x02) &&
		line[1] == 0x00 && line[2] == 0x00 && line[3] == 0x00 {
		return line[8:]
	}
	return line
}
