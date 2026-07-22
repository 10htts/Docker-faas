package gateway

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
)

// asyncCallbackMaxBodyBytes bounds how much of a function's response is buffered
// to deliver to an X-Callback-Url, so a large or streaming response cannot
// exhaust gateway memory across concurrent async invocations. Responses larger
// than this are truncated in the callback body (the function still ran).
const asyncCallbackMaxBodyBytes = 8 * 1024 * 1024

const asyncCallbackTruncatedHeader = "X-Docker-Faas-Callback-Truncated"

func readAsyncCallbackBody(r io.Reader, maxBytes int64) ([]byte, bool, error) {
	if maxBytes < 0 {
		return nil, false, fmt.Errorf("callback body limit must not be negative")
	}
	body, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(body)) <= maxBytes {
		return body, false, nil
	}
	return body[:maxBytes], true, nil
}

// asyncCallbackClient posts async invocation results to X-Callback-Url
// targets. The timeout bounds the callback wait so a slow or dead callback
// receiver can never pin the goroutine (the pinned queue-worker relies on its
// HTTP client timeout the same way).
var asyncCallbackClient = &http.Client{Timeout: 30 * time.Second}

// validateCallbackURL accepts only absolute http/https callback URLs. The
// pinned gateway (openfaas/faas 0.27.13 gateway/handlers/queue_proxy.go)
// answers 400 for unparsable X-Callback-Url values; restricting the scheme to
// http/https is an additional hardening documented in the conformance matrix.
func validateCallbackURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("unable to parse the callback URL: %v", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("callback URL scheme must be http or https")
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("callback URL host is required")
	}
	return parsed, nil
}

// blockInternalCallbackTarget rejects a callback host that is, or resolves to, a
// loopback / link-local / private / cloud-metadata address (SSRF guard, opt-in).
// It reuses the git-URL block list. This is a best-effort resolve-time check and
// is not proof against DNS rebinding; pair it with network egress controls.
func blockInternalCallbackTarget(host string) error {
	if host == "" {
		return fmt.Errorf("callback URL host is required")
	}
	if isBlockedHostname(strings.ToLower(host)) {
		return fmt.Errorf("callback URL host %q is not permitted", host)
	}
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return fmt.Errorf("callback URL host %q resolves to a blocked address", host)
		}
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("callback URL host %q could not be resolved", host)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return fmt.Errorf("callback URL host %q resolves to a blocked address", host)
		}
	}
	return nil
}

// HandleInvokeFunctionAsync handles POST /async-function/{name} and fire-and-forget invocations.
//
// When the caller supplies an X-Callback-Url header, the function's response
// is POSTed to that URL after execution with the headers the pinned
// queue-worker (openfaas/nats-queue-worker 0.14.2) sets: X-Call-Id,
// X-Function-Name, X-Function-Status and X-Duration-Seconds, plus the
// function's own response headers. Callback failures are logged and never
// retried, matching the pinned queue-worker.
func (g *Gateway) HandleInvokeFunctionAsync(w http.ResponseWriter, r *http.Request) {
	functionName := normalizeFunctionName(mux.Vars(r)["name"])
	if err := validateFunctionName(functionName); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	callID, _ := g.stampCallHeaders(w, r)

	var callbackURL *url.URL
	if rawCallback := r.Header.Get("X-Callback-Url"); rawCallback != "" {
		parsed, err := validateCallbackURL(rawCallback)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Opt-in SSRF guard (default off = OpenFaaS-compatible): reject callback
		// hosts that resolve to loopback / link-local / private / metadata IPs.
		if g.asyncCallbackBlockInternal {
			if err := blockInternalCallbackTarget(parsed.Hostname()); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		callbackURL = parsed
	}

	// Get function metadata
	fn, err := g.store.GetFunction(functionName)
	if err != nil {
		http.Error(w, "Function not found", http.StatusNotFound)
		return
	}

	// Register the async invocation as in-flight and HOLD the token for the
	// full lifetime of the background goroutine, so the idle reaper cannot
	// reclaim the function while asynchronous work is still running (SZ-03).
	release, reclaimInProgress := g.trackInvocation(functionName)
	releaseOnce := sync.OnceFunc(release)

	// Check if function needs to scale up from zero
	containers, err := g.provider.GetFunctionContainers(r.Context(), functionName)
	if err != nil {
		releaseOnce()
		g.logger.Errorf("Failed to get containers for function %s: %v", functionName, err)
		http.Error(w, "Failed to get function containers", http.StatusInternalServerError)
		return
	}

	availableReplicas := 0
	for _, c := range containers {
		if strings.Contains(c.Status, "running") || strings.Contains(c.Status, "Up") {
			availableReplicas++
		}
	}

	if availableReplicas == 0 || reclaimInProgress {
		g.logger.Infof("Scaling function %s from zero for async invocation...", functionName)

		// Single-leader cold start (SZ-02). Detached from the request context
		// (RT-219): async is fire-and-forget from the caller's perspective, so a
		// client disconnect must not abort the scale-up; a bounded timeout
		// replaces the request's lifetime.
		coldCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		err := g.ensureReadyFromZero(coldCtx, fn)
		cancel()
		if err != nil {
			releaseOnce()
			g.logger.Errorf("Failed to scale function %s from zero: %v", functionName, err)
			http.Error(w, "Failed to scale function", http.StatusInternalServerError)
			return
		}

		g.logger.Infof("Function %s scaled from zero and ready for async invocation", functionName)
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		releaseOnce()
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	headers := make(http.Header)
	for key, values := range r.Header {
		for _, value := range values {
			headers.Add(key, value)
		}
	}
	headers.Set("X-Call-Id", callID)

	go func(method string, payload []byte, hdr http.Header) {
		defer releaseOnce()
		// The 202 was already written, so the stdlib's per-connection recover no
		// longer shields this goroutine: an unrecovered panic here would kill
		// the whole gateway process (RT-219). Contain it.
		defer func() {
			if rec := recover(); rec != nil {
				g.logger.Errorf("Async invoke panicked for %s: %v", functionName, rec)
			}
		}()
		start := time.Now()

		req, err := http.NewRequestWithContext(context.Background(), method, "/", bytes.NewReader(payload))
		if err != nil {
			g.logger.Errorf("Async invoke failed to create request for %s: %v", functionName, err)
			return
		}
		for key, values := range hdr {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}

		var (
			statusCode      int
			responseBody    []byte
			responseHeaders http.Header
		)

		resp, err := g.router.RouteRequest(context.Background(), functionName, req)
		if err != nil {
			g.logger.Errorf("Async invoke failed for %s: %v", functionName, err)
			// Report the failure to the callback like the pinned queue-worker
			// does: status 500 with the error text as the body.
			statusCode = http.StatusInternalServerError
			responseBody = []byte(err.Error())
		} else {
			statusCode = resp.StatusCode
			responseHeaders = resp.Header.Clone()
			if responseHeaders == nil {
				responseHeaders = make(http.Header)
			}
			if callbackURL != nil {
				// Only buffer the body when it will actually be delivered, and
				// bound it so a large or streaming function response cannot
				// exhaust gateway memory across concurrent async invocations.
				var truncated bool
				responseBody, truncated, err = readAsyncCallbackBody(resp.Body, asyncCallbackMaxBodyBytes)
				if err != nil {
					g.logger.Errorf("Async invoke failed to read response for %s: %v", functionName, err)
					responseBody = nil
				}
				if truncated {
					// The callback request computes its own Content-Length from the
					// bounded body; never forward the function's original full length.
					responseHeaders.Del("Content-Length")
					responseHeaders.Del("Content-Range")
					responseHeaders.Set(asyncCallbackTruncatedHeader, "true")
					g.logger.Warnf("Async callback body for %s truncated at %d bytes", functionName, asyncCallbackMaxBodyBytes)
				}
			}
			// Drain whatever remains without buffering so the connection can be
			// reused and no body is held in memory when there is no callback.
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}

		if callbackURL != nil {
			g.postAsyncCallback(callbackURL, callID, functionName, statusCode, time.Since(start), responseHeaders, responseBody)
		}
	}(r.Method, body, headers)

	g.writeJSON(w, http.StatusAccepted, map[string]string{
		"status": "accepted",
		"callId": callID,
	})
}

// postAsyncCallback POSTs an async invocation result to the callback URL with
// the pinned queue-worker (0.14.2) header set: the function's own response
// headers first, then X-Call-Id, X-Function-Name, X-Function-Status and
// X-Duration-Seconds. Failures are logged, never retried.
func (g *Gateway) postAsyncCallback(callbackURL *url.URL, callID, functionName string, statusCode int, duration time.Duration, functionHeaders http.Header, body []byte) {
	req, err := http.NewRequest(http.MethodPost, callbackURL.String(), bytes.NewReader(body))
	if err != nil {
		g.logger.Errorf("Async callback for %s: unable to build request to %s: %v", functionName, callbackURL, err)
		return
	}

	for key, values := range functionHeaders {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	if callID != "" {
		req.Header.Set("X-Call-Id", callID)
	}
	req.Header.Set("X-Function-Name", functionName)
	req.Header.Set("X-Function-Status", fmt.Sprintf("%d", statusCode))
	req.Header.Set("X-Duration-Seconds", fmt.Sprintf("%f", duration.Seconds()))

	resp, err := g.asyncCallbackHTTPClient().Do(req)
	if err != nil {
		g.logger.Errorf("Error posting to callback-url %s for %s: %v", callbackURL, functionName, err)
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	g.logger.Infof("Async callback for %s posted to %s: %d", functionName, callbackURL, resp.StatusCode)
}

// asyncCallbackHTTPClient returns the client used to POST callbacks. When the
// SSRF guard is enabled it re-validates EVERY redirect hop, so a permitted
// public callback cannot 30x-redirect to a loopback / private / metadata target
// (the initial-host check alone is redirect-bypassable). When disabled it
// returns the shared default client (OpenFaaS-compatible: redirects followed).
func (g *Gateway) asyncCallbackHTTPClient() *http.Client {
	if !g.asyncCallbackBlockInternal {
		return asyncCallbackClient
	}
	return &http.Client{
		Timeout: asyncCallbackClient.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return blockInternalCallbackTarget(req.URL.Hostname())
		},
	}
}
