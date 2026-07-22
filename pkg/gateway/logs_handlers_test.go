package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/docker-faas/docker-faas/pkg/provider"
	"github.com/docker-faas/docker-faas/pkg/types"
)

// fakeStreamingProvider implements the optional functionLogStreamer capability
// on top of the base fakeProvider.
type fakeStreamingProvider struct {
	fakeProvider
	entries   []provider.LogEntry
	streamErr error

	gotName   string
	gotSince  time.Time
	gotTail   int
	gotFollow bool
}

func (p *fakeStreamingProvider) StreamFunctionLogs(ctx context.Context, functionName string, since time.Time, tail int, follow bool) (<-chan provider.LogEntry, error) {
	p.gotName = functionName
	p.gotSince = since
	p.gotTail = tail
	p.gotFollow = follow
	if p.streamErr != nil {
		return nil, p.streamErr
	}
	out := make(chan provider.LogEntry, len(p.entries))
	for _, entry := range p.entries {
		out <- entry
	}
	close(out)
	return out, nil
}

func logsStore() *fakeStore {
	return &fakeStore{functions: map[string]*types.FunctionMetadata{
		"hello": {Name: "hello", Image: "example/hello:latest", Replicas: 1},
	}}
}

func decodeNDJSON(t *testing.T, body string) []logMessage {
	t.Helper()
	// Decode exactly the way faas-cli 0.18.0 proxy/logs.go does: a streaming
	// json.Decoder loop over the response body.
	decoder := json.NewDecoder(strings.NewReader(body))
	var messages []logMessage
	for decoder.More() {
		var msg logMessage
		if err := decoder.Decode(&msg); err != nil {
			t.Fatalf("failed to decode NDJSON message: %v (body %q)", err, body)
		}
		messages = append(messages, msg)
	}
	return messages
}

func TestHandleGetLogs_StreamsPinnedNDJSON(t *testing.T) {
	ts1 := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	ts2 := ts1.Add(time.Second)
	fp := &fakeStreamingProvider{entries: []provider.LogEntry{
		{Name: "hello", Instance: "hello-0", Timestamp: ts1, Text: "line one"},
		{Name: "hello", Instance: "hello-1", Timestamp: ts2, Text: "line two"},
	}}
	gw := newTestGateway(logsStore(), fp, &fakeRouter{})

	req := httptest.NewRequest(http.MethodGet,
		"/system/logs?name=hello&tail=5&follow=false&since=2026-07-21T09:00:00Z", nil)
	recorder := httptest.NewRecorder()
	gw.HandleGetLogs(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if ct := recorder.Header().Get("Content-Type"); ct != "application/x-ndjson" {
		t.Fatalf("expected content-type application/x-ndjson, got %q", ct)
	}

	if fp.gotName != "hello" || fp.gotTail != 5 || fp.gotFollow {
		t.Fatalf("expected streamer to receive name=hello tail=5 follow=false, got %q/%d/%v",
			fp.gotName, fp.gotTail, fp.gotFollow)
	}
	wantSince := time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)
	if !fp.gotSince.Equal(wantSince) {
		t.Fatalf("expected since %v, got %v", wantSince, fp.gotSince)
	}

	messages := decodeNDJSON(t, recorder.Body.String())
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d: %s", len(messages), recorder.Body.String())
	}
	first := messages[0]
	if first.Name != "hello" || first.Namespace != DefaultFunctionNamespace ||
		first.Instance != "hello-0" || !first.Timestamp.Equal(ts1) || first.Text != "line one" {
		t.Fatalf("unexpected first message: %+v", first)
	}
	if messages[1].Text != "line two" || messages[1].Instance != "hello-1" {
		t.Fatalf("unexpected second message: %+v", messages[1])
	}

	// Every line must carry all pinned keys, no omissions.
	line := strings.SplitN(strings.TrimSpace(recorder.Body.String()), "\n", 2)[0]
	for _, key := range []string{`"name"`, `"namespace"`, `"instance"`, `"timestamp"`, `"text"`} {
		if !strings.Contains(line, key) {
			t.Fatalf("expected NDJSON line to contain %s, got %s", key, line)
		}
	}
}

func TestHandleGetLogs_FiltersByInstance(t *testing.T) {
	ts := time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	fp := &fakeStreamingProvider{entries: []provider.LogEntry{
		{Name: "hello", Instance: "hello-0", Timestamp: ts, Text: "keep"},
		{Name: "hello", Instance: "hello-1", Timestamp: ts, Text: "drop"},
	}}
	gw := newTestGateway(logsStore(), fp, &fakeRouter{})

	req := httptest.NewRequest(http.MethodGet, "/system/logs?name=hello&instance=hello-0", nil)
	recorder := httptest.NewRecorder()
	gw.HandleGetLogs(recorder, req)

	messages := decodeNDJSON(t, recorder.Body.String())
	if len(messages) != 1 || messages[0].Text != "keep" {
		t.Fatalf("expected only instance hello-0 messages, got %+v", messages)
	}
}

func TestHandleGetLogs_ParseFailures422(t *testing.T) {
	gw := newTestGateway(logsStore(), &fakeStreamingProvider{}, &fakeRouter{})

	cases := []string{
		"/system/logs",                          // missing name
		"/system/logs?name=hello&tail=abc",      // bad tail
		"/system/logs?name=hello&follow=maybe",  // bad follow
		"/system/logs?name=hello&since=lastday", // bad since
		"/system/logs?name=-bad-",               // invalid function name
	}
	for _, target := range cases {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		recorder := httptest.NewRecorder()
		gw.HandleGetLogs(recorder, req)
		if recorder.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected status %d for %q, got %d", http.StatusUnprocessableEntity, target, recorder.Code)
		}
	}
}

func TestHandleGetLogs_UnknownFunction404(t *testing.T) {
	gw := newTestGateway(&fakeStore{functions: map[string]*types.FunctionMetadata{}}, &fakeStreamingProvider{}, &fakeRouter{})

	req := httptest.NewRequest(http.MethodGet, "/system/logs?name=nope", nil)
	recorder := httptest.NewRecorder()
	gw.HandleGetLogs(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, recorder.Code)
	}
}

func TestHandleGetLogs_UnknownNamespace400(t *testing.T) {
	gw := newTestGateway(logsStore(), &fakeStreamingProvider{}, &fakeRouter{})

	req := httptest.NewRequest(http.MethodGet, "/system/logs?name=hello&namespace=prod", nil)
	recorder := httptest.NewRecorder()
	gw.HandleGetLogs(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}
}

func TestHandleGetLogs_NormalizesNameSuffix(t *testing.T) {
	fp := &fakeStreamingProvider{}
	gw := newTestGateway(logsStore(), fp, &fakeRouter{})

	req := httptest.NewRequest(http.MethodGet, "/system/logs?name=hello.openfaas-fn", nil)
	recorder := httptest.NewRecorder()
	gw.HandleGetLogs(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if fp.gotName != "hello" {
		t.Fatalf("expected suffix-stripped name hello, got %q", fp.gotName)
	}
}

func TestHandleGetLogs_FallbackParsesLegacyBlob(t *testing.T) {
	// Base fakeProvider does NOT implement functionLogStreamer, forcing the
	// GetContainerLogs fallback. The blob mixes a docker multiplex frame
	// header, an RFC3339Nano timestamp prefix, and a bare line.
	frameHeader := string([]byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10})
	fp := &fakeProvider{
		logs: "2026-07-21T10:00:00.123456789Z timestamped line\n" +
			frameHeader + "framed line\n" +
			"bare line\n",
	}
	gw := newTestGateway(logsStore(), fp, &fakeRouter{})

	req := httptest.NewRequest(http.MethodGet, "/system/logs?name=hello", nil)
	recorder := httptest.NewRecorder()
	gw.HandleGetLogs(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if ct := recorder.Header().Get("Content-Type"); ct != "application/x-ndjson" {
		t.Fatalf("expected content-type application/x-ndjson, got %q", ct)
	}

	messages := decodeNDJSON(t, recorder.Body.String())
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d: %s", len(messages), recorder.Body.String())
	}
	wantTS := time.Date(2026, 7, 21, 10, 0, 0, 123456789, time.UTC)
	if messages[0].Text != "timestamped line" || !messages[0].Timestamp.Equal(wantTS) {
		t.Fatalf("expected parsed timestamped line, got %+v", messages[0])
	}
	if messages[1].Text != "framed line" {
		t.Fatalf("expected frame header stripped, got %+v", messages[1])
	}
	if messages[2].Text != "bare line" || messages[2].Timestamp.IsZero() {
		t.Fatalf("expected bare line with best-effort timestamp, got %+v", messages[2])
	}
	for _, msg := range messages {
		if msg.Name != "hello" || msg.Namespace != DefaultFunctionNamespace {
			t.Fatalf("expected name/namespace on every message, got %+v", msg)
		}
	}
}

func TestParseDockerLogLine(t *testing.T) {
	ts, text := provider.ParseDockerLogLine("2026-07-21T10:00:00.5Z hello world")
	if text != "hello world" {
		t.Fatalf("expected text %q, got %q", "hello world", text)
	}
	want := time.Date(2026, 7, 21, 10, 0, 0, 500000000, time.UTC)
	if !ts.Equal(want) {
		t.Fatalf("expected timestamp %v, got %v", want, ts)
	}

	ts, text = provider.ParseDockerLogLine("no timestamp here")
	if !ts.IsZero() || text != "no timestamp here" {
		t.Fatalf("expected verbatim line with zero timestamp, got %v %q", ts, text)
	}
}

func TestStripDockerFrameHeader(t *testing.T) {
	framed := string([]byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x05}) + "oops"
	if got := stripDockerFrameHeader(framed); got != "oops" {
		t.Fatalf("expected frame header stripped, got %q", got)
	}
	if got := stripDockerFrameHeader("plain line"); got != "plain line" {
		t.Fatalf("expected plain line untouched, got %q", got)
	}
	short := string([]byte{0x01, 0x00, 0x00})
	if got := stripDockerFrameHeader(short); got != short {
		t.Fatalf("expected short line untouched, got %q", got)
	}
}
