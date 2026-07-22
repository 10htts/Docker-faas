package gateway

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/docker-faas/docker-faas/pkg/types"
	"github.com/gorilla/mux"
)

type closeSignalReadCloser struct {
	io.Reader
	closed chan struct{}
}

func (r *closeSignalReadCloser) Close() error {
	close(r.closed)
	return nil
}

func TestReadAsyncCallbackBodyBoundsAndSignalsTruncation(t *testing.T) {
	body, truncated, err := readAsyncCallbackBody(strings.NewReader("abcdef"), 4)
	if err != nil {
		t.Fatalf("read bounded callback body: %v", err)
	}
	if string(body) != "abcd" || !truncated {
		t.Fatalf("got body=%q truncated=%v, want %q/true", body, truncated, "abcd")
	}

	body, truncated, err = readAsyncCallbackBody(strings.NewReader("abcd"), 4)
	if err != nil {
		t.Fatalf("read exact-size callback body: %v", err)
	}
	if string(body) != "abcd" || truncated {
		t.Fatalf("got body=%q truncated=%v, want %q/false", body, truncated, "abcd")
	}
}

func TestHandleInvokeFunctionAsyncWithoutCallbackDrainsResponse(t *testing.T) {
	responseBuffer := bytes.NewBufferString("discarded async response")
	closed := make(chan struct{})
	response := &http.Response{
		StatusCode: http.StatusOK,
		Body:       &closeSignalReadCloser{Reader: responseBuffer, closed: closed},
	}
	gw := newTestGateway(
		&fakeStore{functions: map[string]*types.FunctionMetadata{
			"hello": {Name: "hello", Image: "alpine:latest", Replicas: 1},
		}},
		&fakeProvider{containers: []*types.Container{{Name: "hello", Status: "running"}}},
		&fakeRouter{resp: response},
	)

	req := httptest.NewRequest(http.MethodPost, "/async-function/hello", strings.NewReader("ping"))
	req = mux.SetURLVars(req, map[string]string{"name": "hello"})
	recorder := httptest.NewRecorder()
	gw.HandleInvokeFunctionAsync(recorder, req)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, recorder.Code)
	}

	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("async response body was not drained and closed")
	}
	if responseBuffer.Len() != 0 {
		t.Fatalf("async response body was not fully drained: %d bytes remain", responseBuffer.Len())
	}
}

// TestBlockInternalCallbackTarget covers the opt-in SSRF guard for async
// X-Callback-Url targets (default-off; exercised here directly). Literal IPs
// avoid DNS and keep the test hermetic.
func TestBlockInternalCallbackTarget(t *testing.T) {
	blocked := []string{
		"127.0.0.1",         // loopback
		"169.254.169.254",   // cloud metadata (link-local)
		"10.1.2.3",          // RFC1918
		"192.168.0.5",       // RFC1918
		"172.16.9.9",        // RFC1918
		"0.0.0.0",           // unspecified
		"localhost",         // blocked hostname
		"metadata.internal", // .internal suffix
		"db.local",          // .local suffix
	}
	for _, h := range blocked {
		if err := blockInternalCallbackTarget(h); err == nil {
			t.Errorf("host %q must be blocked as an internal SSRF target", h)
		}
	}

	allowed := []string{
		"93.184.216.34", // public IP (example.net range)
		"8.8.8.8",       // public IP
	}
	for _, h := range allowed {
		if err := blockInternalCallbackTarget(h); err != nil {
			t.Errorf("public host %q must be allowed, got %v", h, err)
		}
	}

	if err := blockInternalCallbackTarget(""); err == nil {
		t.Error("empty host must be rejected")
	}
}

// TestAsyncCallbackClientRevalidatesRedirects proves the RT (SSRF-redirect) fix:
// with the guard enabled, the callback client must re-validate each redirect hop
// and refuse a 30x to an internal target — the initial-host check alone is
// redirect-bypassable.
func TestAsyncCallbackClientRevalidatesRedirects(t *testing.T) {
	// A public-looking server that redirects to a blocked (loopback) target.
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:9/secret", http.StatusFound)
	}))
	defer redirector.Close()

	// Guard ON: the redirect to 127.0.0.1 must be refused.
	gwOn := &Gateway{asyncCallbackBlockInternal: true}
	req, _ := http.NewRequest(http.MethodPost, redirector.URL, nil)
	if _, err := gwOn.asyncCallbackHTTPClient().Do(req); err == nil {
		t.Fatal("guard on: a redirect to an internal target must be refused")
	}

	// Guard OFF: default OpenFaaS behavior — redirects are followed (the target
	// 127.0.0.1:9 refuses the connection, which is a transport error, not a
	// policy refusal; either way the client is the shared default one).
	gwOff := &Gateway{asyncCallbackBlockInternal: false}
	if gwOff.asyncCallbackHTTPClient() != asyncCallbackClient {
		t.Fatal("guard off must use the shared default client (redirects followed)")
	}
}

// TestValidateCallbackURLSchemeAndHost pins the scheme/host validation that runs
// regardless of the SSRF opt-in.
func TestValidateCallbackURLSchemeAndHost(t *testing.T) {
	bad := []string{
		"ftp://example.com/x",
		"file:///etc/passwd",
		"gopher://example.com",
		"http://",
		"://noscheme",
	}
	for _, raw := range bad {
		if _, err := validateCallbackURL(raw); err == nil {
			t.Errorf("callback URL %q must be rejected", raw)
		}
	}
	for _, raw := range []string{"http://example.com/cb", "https://hooks.example.com/a/b"} {
		if _, err := validateCallbackURL(raw); err != nil {
			t.Errorf("valid callback URL %q rejected: %v", raw, err)
		}
	}
}
